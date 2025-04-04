package ioc

import "go.uber.org/zap"

func InitLogger() *zap.Logger {
	lg, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	return lg
}
