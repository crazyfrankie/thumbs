package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/crazyfrankie/thumbs/rpc"
)

func main() {
	server := rpc.NewServer()

	g := &run.Group{}

	g.Add(func() error {
		err := server.Serve()
		return err
	}, func(err error) {
		server.GracefulStop()
		server.Stop()
	})

	thumbServer := &http.Server{Addr: ":9091"}
	g.Add(func() error {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(
			rpc.ThumbsReg,
			promhttp.HandlerOpts{
				EnableOpenMetrics: true,
			},
		))
		thumbServer.Handler = mux
		return thumbServer.ListenAndServe()
	}, func(err error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()

		if err := thumbServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("failed to shutdown metrics server: %v", err)
		}
	})

	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	if err := g.Run(); err != nil {
		log.Printf("program interrupted, err:%s", err)
		return
	}
}
