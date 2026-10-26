package mysql

import (
	"context"

	"github.com/go-xorm/xorm"

	"wafadmin/internal/model"
	"wafadmin/pkg/sqlwatch"
)

// settingRepo 是 SettingRepository 的 MySQL 实现。
type settingRepo struct {
	engine   *xorm.Engine
	observer *sqlwatch.Observer
}

// All 返回全部配置项。
func (r *settingRepo) All(ctx context.Context) (map[string]string, error) {
	defer r.observer.Trace(ctx, "setting.all", "SELECT config_key, config_value FROM setting")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	items := make([]model.Setting, 0, 16)
	if err := r.engine.Find(&items); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		out[item.ConfigKey] = item.ConfigValue
	}
	return out, nil
}

// Get 读取单个配置项。
func (r *settingRepo) Get(ctx context.Context, key string) (string, bool, error) {
	defer r.observer.Trace(ctx, "setting.get", "SELECT config_value FROM setting WHERE config_key = ?")()
	if err := ctx.Err(); err != nil {
		return "", false, err
	}

	item := new(model.Setting)
	has, err := r.engine.Id(key).Get(item)
	if err != nil {
		return "", false, err
	}
	if !has {
		return "", false, nil
	}
	return item.ConfigValue, true, nil
}

// Set 写入或覆盖配置项。
func (r *settingRepo) Set(ctx context.Context, key, value string) error {
	defer r.observer.Trace(ctx, "setting.set", "REPLACE INTO setting (config_key, config_value) VALUES (?, ?)")()
	if err := ctx.Err(); err != nil {
		return err
	}

	item := &model.Setting{ConfigKey: key, ConfigValue: value}
	_, err := r.engine.Insert(item)
	return err
}

// Delete 删除配置项。
func (r *settingRepo) Delete(ctx context.Context, key string) error {
	defer r.observer.Trace(ctx, "setting.delete", "DELETE FROM setting WHERE config_key = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := r.engine.Id(key).Delete(new(model.Setting))
	return err
}
