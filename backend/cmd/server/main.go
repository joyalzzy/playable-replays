package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/api"
	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
	"github.com/joyalzzy/playable-replays/backend/internal/opponent"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	fixturePath := env("FIXTURE_PATH", "../fixtures/moments.json")
	moments, err := fixtures.Load(fixturePath)
	if err != nil {
		logger.Error("load fixtures", "error", err)
		os.Exit(1)
	}
	var opponentModel engine.OpponentPositionModel
	if endpoint := os.Getenv("OPPONENT_MODEL_URL"); endpoint != "" {
		modelName := os.Getenv("OPPONENT_MODEL_NAME")
		modelVersion := os.Getenv("OPPONENT_MODEL_VERSION")
		opponentModel, err = opponent.NewHTTPModel(endpoint, modelName, modelVersion, nil)
		if err != nil {
			logger.Error("configure opponent model", "error", err)
			os.Exit(1)
		}
		logger.Info("opponent position model enabled", "model", modelName, "version", modelVersion)
	}

	server := &http.Server{
		Addr:              env("LISTEN_ADDR", "127.0.0.1:8080"),
		Handler:           api.NewWithOpponentModel(moments, logger, opponentModel).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}

	go func() {
		logger.Info("API listening", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown", "error", err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
