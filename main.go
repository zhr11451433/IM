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

	navHTTP "IM/services/navigator/routers"
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

	//r := appHTTP.SetRouter(gormDB, rt)
	servers := []*http.Server{
		{
			Addr:    fmt.Sprintf(":%d", cfg.API.HTTPPort),
			Handler: appHTTP.SetRouter(gormDB, rt),
		},
		{
			Addr:    fmt.Sprintf(":%d", cfg.Nav.HTTPPort),
			Handler: navHTTP.SetRouter(gormDB, cfg.Nav.Servers),
		},
	}

	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	// 每个服务都可能报告错误，给每个服务留一个位置。
	errCh := make(chan error, len(servers))

	for _, srv := range servers {
		go func(s *http.Server) {
			err := s.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("HTTP %s: %w", s.Addr, err)
			}
		}(srv)
	}

	select {
	case err := <-errCh:
		Log.Printf("server failed: %v", err)
	case <-ctx.Done():
		Log.Println("shutting down")
	}

	// 任意服务失败或收到退出信号，都关闭所有服务。
	for _, srv := range servers {
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		err := srv.Shutdown(shutdownCtx)
		cancel()

		if err != nil {
			Log.Printf("shutdown %s: %v", srv.Addr, err)
			if closeErr := srv.Close(); closeErr != nil {
				Log.Printf("close %s: %v", srv.Addr, closeErr)
			}
		}
	}
}
