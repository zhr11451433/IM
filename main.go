package main

import (
	"IM/commons/actor"
	"IM/commons/db"
	appHTTP "IM/services/apigateway/routers"
	"IM/services/services/usermanager"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"IM/commons/config"
	"IM/commons/log"

	Log "log"
)

func main() {
	cfg, err := config.Load("conf/config.yml")
	if err != nil {
		fmt.Println("load config failed:", err)
		os.Exit(1)
	}
	if err := log.Init(cfg.Log.LogPath, cfg.Log.LogName); err != nil {
		fmt.Println("init log failed:", err)
		os.Exit(1)
	}
	gormDB, err := db.OpenGorm(cfg.Mysql)
	if err != nil {
		log.Logger.Error().Err(err).Msg("init gorm failed")
		os.Exit(1)
	}
	defer func() {
		sqlDB, err := gormDB.DB()
		if err != nil {
			log.Logger.Error().Err(err).Msg("gorm sql db")
			return
		}
		if err := sqlDB.Close(); err != nil {
			log.Logger.Error().Err(err).Msg("close gorm")
		}
	}()
	log.Logger.Info().Str("node", cfg.NodeName).Msg("mysql connected")

	rt := actor.New()
	if err := usermanager.Register(rt, gormDB); err != nil {
		log.Logger.Error().Err(err).Msg("register actors")
		os.Exit(1)
	}
	r := appHTTP.SetRouter(gormDB, rt)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.API.HTTPPort),
		Handler: r,
	}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errCh:
		Log.Fatalf("routers server error: %v", err)
	case <-ctx.Done():
		Log.Println("shutting down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		Log.Fatalf("shutdown: %v", err)
	}
}
