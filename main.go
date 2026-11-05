package main

import (
	"IM/commons/actor"
	"IM/commons/db"
	appHTTP "IM/services/apigateway/routers"
	"IM/services/usermanager"
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

	wsServer "IM/services/connectmanager/server"
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
	//服务器启动时，先把所有业务岗位安排好；之后请求来了，再把工作分配给对应岗位
	if err := usermanager.Register(rt, gormDB); err != nil {
		log.Logger.Error().Err(err).Msg("register actors")
		os.Exit(1)
	}
	ws, err := wsServer.New(
		func(ctx context.Context, appKey string) (wsServer.AuthApp, bool, error) {
			gdb := gormDB.WithContext(ctx)
			key, found, err := db.FindAppSecureKey(gdb, appKey)
			if err != nil || !found {
				return wsServer.AuthApp{}, found, err
			}
			minutes, err := db.FindTokenEffectiveMinute(gdb, appKey)
			if err != nil {
				return wsServer.AuthApp{}, false, err
			}
			return wsServer.AuthApp{
				SecureKey: key, EffectiveMinutes: minutes,
			}, true, nil
		},
		wsServer.Options{
			ConnectTimeout: time.Duration(
				cfg.Connect.ConnectTimeoutSeconds) * time.Second,
			HeartbeatInterval: time.Duration(
				cfg.Connect.HeartbeatIntervalSeconds) * time.Second,
			HeartbeatTimeout: time.Duration(
				cfg.Connect.HeartbeatTimeoutSeconds) * time.Second,
		},
	)
	if err != nil {
		Log.Printf("init websocket: %v", err)
		return
	}
	defer ws.Close()
	//HTTP 路由分发器（ServeMux）。
	//它负责根据客户端请求的 URL 路径，把请求交给对应的处理函数。
	//你可以把它理解成一个路由表，专门记录：
	//- 请求 /im，交给谁处理？
	//- 请求 /login，交给谁处理？
	//- 请求 /upload，交给谁处理？
	wsMux := http.NewServeMux()
	wsMux.HandleFunc("/im", ws.ImWsServer)
	//r	 := appHTTP.SetRouter(gormDB, rt)
	servers := []*http.Server{
		{
			Addr:    fmt.Sprintf(":%d", cfg.API.HTTPPort),
			Handler: appHTTP.SetRouter(gormDB, rt),
		},
		{
			Addr:    fmt.Sprintf(":%d", cfg.Nav.HTTPPort),
			Handler: navHTTP.SetRouter(gormDB, cfg.Nav.Servers),
		},
		{
			Addr:    fmt.Sprintf(":%d", cfg.Connect.WSPort),
			Handler: wsMux,
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
	ws.Close()
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
