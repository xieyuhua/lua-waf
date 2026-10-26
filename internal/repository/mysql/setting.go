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

// Set 写入或覆盖配置项（先更新，未命中再插入；并发撞主键时退化为更新）。
func (r *settingRepo) Set(ctx context.Context, key, value string) error {
	defer r.observer.Trace(ctx, "setting.set", "UPDATE setting SET config_value = ? WHERE config_key = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}

	affected, err := r.engine.Table(new(model.Setting)).
		Where("config_key = ?", key).
		Update(&model.Setting{ConfigValue: value})
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}

	// 记录不存在：插入；并发下可能已被别的请求插入，失败时再更新一次
	if _, insertErr := r.engine.Insert(&model.Setting{ConfigKey: key, ConfigValue: value}); insertErr != nil {
		if retry, retryErr := r.engine.Table(new(model.Setting)).
			Where("config_key = ?", key).
			Update(&model.Setting{ConfigValue: value}); retryErr == nil && retry > 0 {
			return nil
		}
		return insertErr
	}
	return nil
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
