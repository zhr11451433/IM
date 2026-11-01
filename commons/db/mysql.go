package db

import (
	"database/sql"
	"fmt"
	"time"

	"IM/commons/config"

	_ "github.com/go-sql-driver/mysql"
)

func Open(cfg config.Mysql) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&timeout=3s",
		cfg.User, cfg.Password, cfg.Address, cfg.Name)
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	conn.SetConnMaxLifetime(time.Minute)
	conn.SetMaxOpenConns(5)
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return conn, nil
}
