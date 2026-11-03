package db

import (
	"IM/commons/config"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

//保存应用的基本信息和认证密钥

type App struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	AppKey       string    `gorm:"column:app_key;size:45;not null;uniqueIndex"`
	AppSecret    string    `gorm:"column:app_secret;size:45;not null"`
	CreatedTime  time.Time `gorm:"column:created_time;autoCreateTime:milli"`
	AppSecureKey string    `gorm:"column:app_secure_key;size:45;not null"`
	UpdatedTime  time.Time `gorm:"column:updated_time;autoUpdateTime:milli"`
}

func (App) TableName() string { return "apps" }

var models = []any{
	&App{},
	&User{},
	&AppExt{},
}

func OpenGorm(cfg config.Mysql) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&timeout=3s",
		cfg.User, cfg.Password, cfg.Address, cfg.Name)
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open gorm: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	if err := gdb.AutoMigrate(models...); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	return gdb, nil
}

func FindAppSecret(gdb *gorm.DB, appKey string) (string, bool, error) {
	var app App
	err := gdb.Where("app_key = ?", appKey).Take(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return app.AppSecret, true, nil
}

func FindAppSecureKey(gdb *gorm.DB, appKey string) (string, bool, error) {
	var app App
	err := gdb.Where("app_key = ?", appKey).Take(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return app.AppSecureKey, true, nil
}
