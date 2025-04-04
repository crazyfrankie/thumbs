package rpc

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/prometheus/client_golang/prometheus"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/crazyfrankie/thumbs/config"
	"github.com/crazyfrankie/thumbs/ioc"
)

var (
	serviceName   = "service/thumbs"
	ThumbsReg     = prometheus.NewRegistry()
	thumbsMetrics = grpcprom.NewServerMetrics()
)

type Server struct {
	*grpc.Server
	port string

	registry *clientv3.Client
	em       endpoints.Manager
	leaseID  clientv3.LeaseID
	ttl      int64
}

func NewServer() *Server {
	registry := ioc.InitRegistry()
	logger := ioc.InitLogger()

	logTraceID := func(ctx context.Context) logging.Fields {
		if span := oteltrace.SpanContextFromContext(ctx); span.IsSampled() {
			return logging.Fields{"traceID", span.TraceID().String()}
		}
		return nil
	}

	labelsFromContext := func(ctx context.Context) prometheus.Labels {
		if span := oteltrace.SpanContextFromContext(ctx); span.IsSampled() {
			return prometheus.Labels{"traceID": span.TraceID().String()}
		}
		return nil
	}

	ThumbsReg.MustRegister(thumbsMetrics)
	// 设置 OpenTelemetry
	tp := initTracerProvider("thumbs-up")
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	s := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			thumbsMetrics.UnaryServerInterceptor(grpcprom.WithExemplarFromContext(labelsFromContext)),
			logging.UnaryServerInterceptor(interceptorLogger(logger), logging.WithFieldsFromContext(logTraceID)),
		),
		grpc.ChainStreamInterceptor(
			thumbsMetrics.StreamServerInterceptor(grpcprom.WithExemplarFromContext(labelsFromContext)),
			logging.StreamServerInterceptor(interceptorLogger(logger), logging.WithFieldsFromContext(logTraceID)),
		))

	return &Server{
		Server:   s,
		port:     config.GetConf().Server.Port,
		ttl:      config.GetConf().Server.TTL,
		registry: registry,
	}
}

func (s *Server) Serve() error {
	conn, err := net.Listen("tcp", s.port)
	if err != nil {
		return err
	}

	err = s.register()
	if err != nil {
		return err
	}

	return s.Server.Serve(conn)
}

func (s *Server) register() error {
	var err error
	s.em, err = endpoints.NewManager(s.registry, serviceName)
	if err != nil {
		return err
	}

	addr := serviceAddr(s.port)
	svcKey := serviceKey(addr)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, err := s.registry.Grant(ctx, s.ttl)
	if err != nil {
		return err
	}
	s.leaseID = lease.ID

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = s.em.AddEndpoint(ctx, svcKey, endpoints.Endpoint{Addr: addr}, clientv3.WithLease(s.leaseID))
	if err != nil {
		return err
	}

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, err := s.registry.KeepAlive(ctx, s.leaseID)
		if err != nil {
			log.Printf("keep alive failed lease id:%d", s.leaseID)
			return
		}
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					log.Println("KeepAlive channel closed")
					return
				}
				fmt.Println("Lease renewed")
			case <-ctx.Done():
				return
			}
		}
	}()

	return err
}

func (s *Server) unRegister() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	if err := s.em.DeleteEndpoint(ctx, serviceKey(serviceAddr(s.port))); err != nil {
		return fmt.Errorf("failed to delete endpoint: %v", err)
	}

	if _, err := s.registry.Revoke(ctx, s.leaseID); err != nil {
		return fmt.Errorf("failed to revoke lease: %v", err)
	}

	return nil
}

func serviceKey(addr string) string {
	return fmt.Sprintf("%s/%s", serviceName, addr)
}

func serviceAddr(port string) string {
	return fmt.Sprintf("%s%s", "127.0.0.1", port)
}

// interceptorLogger adapts zap logger to interceptor logger.
// This code is simple enough to be copied and not imported.
func interceptorLogger(l *zap.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make([]zap.Field, 0, len(fields)/2)

		for i := 0; i < len(fields); i += 2 {
			key := fields[i]
			value := fields[i+1]

			switch v := value.(type) {
			case string:
				f = append(f, zap.String(key.(string), v))
			case int:
				f = append(f, zap.Int(key.(string), v))
			case bool:
				f = append(f, zap.Bool(key.(string), v))
			default:
				f = append(f, zap.Any(key.(string), v))
			}
		}

		logger := l.WithOptions(zap.AddCallerSkip(1)).With(f...)

		switch lvl {
		case logging.LevelDebug:
			logger.Debug(msg)
		case logging.LevelInfo:
			logger.Info(msg)
		case logging.LevelWarn:
			logger.Warn(msg)
		case logging.LevelError:
			logger.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}

func initTracerProvider(servicename string) *trace.TracerProvider {
	res, err := newResource(servicename, "v0.0.1")
	if err != nil {
		fmt.Printf("failed create resource, %s", err)
	}

	tp, err := newTraceProvider(res)
	if err != nil {
		panic(err)
	}

	return tp
}

func newResource(servicename, serviceVersion string) (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceNameKey.String(servicename),
			semconv.ServiceVersionKey.String(serviceVersion)))
}

func newTraceProvider(res *resource.Resource) (*trace.TracerProvider, error) {
	exporter, err := zipkin.New("http://localhost:9411/api/v2/spans")
	if err != nil {
		return nil, err
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter, trace.WithBatchTimeout(time.Second)), trace.WithResource(res))

	return traceProvider, nil
}
