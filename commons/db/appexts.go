package db

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

//保存应用的扩展配置，允许每个应用存储不同的自定义数据

//app-a 最大上传 10MB。
//app-b 最大上传 50MB。
//app-a 允许视频。
//app-b 禁止视频。

type AppExt struct {
	ID int64 `gorm:"primaryKey;autoIncrement"`
	//属于哪个应用
	AppKey string `gorm:"column:app_key;size:50;uniqueIndex:idx_app_item"`
	//扩展配置的名称
	AppItemKey string `gorm:"column:app_item_key;size:50;uniqueIndex:idx_app_item"`
	//扩展配置的值
	AppItemValue string    `gorm:"column:app_item_value;type:text"`
	UpdatedTime  time.Time `gorm:"column:updated_time;autoUpdateTime:milli"`
}

func (AppExt) TableName() string { return "appexts" }

func FindTokenEffectiveMinute(gdb *gorm.DB, appKey string) (int, error) {
	var appExt AppExt
	if err := gdb.Where("app_key = ? and app_item_key = ?", appKey, "token_effective_minute").Take(&appExt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	appItemValue, err := strconv.Atoi(strings.TrimSpace(appExt.AppItemValue))
	if err != nil {
		return 0, fmt.Errorf("invalid token_effective_minute: %w", err)
	}
	// time.Duration 用 int64 纳秒表示，限制分钟数以避免乘法溢出。
	const maxMinutes = int64((1<<63 - 1) / int64(time.Minute))

	if appItemValue < 0 || int64(appItemValue) > maxMinutes {
		return 0, fmt.Errorf(
			"token_effective_minute must be between 0 and %d",
			maxMinutes,
		)
	}
	return appItemValue, nil
}
