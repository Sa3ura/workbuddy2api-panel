// health_config_test.go 上游健康巡检的配置契约。
//
// 语义与 task_sweep 完全同构：enabled 决定启不启动，minutes<=0 回落默认 360（6h），
// 关闭时 interval 必须保持 0（不启动循环），而不是留一个默认值让调度器空转。
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 键缺席 → 默认开启、360 分钟。
func TestHealthCheckDefault(t *testing.T) {
	c := Default()
	if err := c.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if !c.Schedule.HealthCheckEnabled {
		t.Error("health_check_enabled 缺省应为 true")
	}
	if want := 360 * time.Minute; c.HealthCheckInterval != want {
		t.Errorf("HealthCheckInterval=%v want %v", c.HealthCheckInterval, want)
	}
}

// 显式配置覆盖默认。
func TestHealthCheckParsedFromFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "c.json")
	if err := os.WriteFile(fp, []byte(`{"schedule":{"health_check_minutes":30}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(fp)
	if err != nil {
		t.Fatal(err)
	}
	if want := 30 * time.Minute; c.HealthCheckInterval != want {
		t.Errorf("HealthCheckInterval=%v want %v", c.HealthCheckInterval, want)
	}
}

// minutes<=0 → 回落 360（不是「关闭」：关闭要显式 enabled=false）。
func TestHealthCheckZeroFallsBack(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "c.json")
	if err := os.WriteFile(fp, []byte(`{"schedule":{"health_check_minutes":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(fp)
	if err != nil {
		t.Fatal(err)
	}
	if want := 360 * time.Minute; c.HealthCheckInterval != want {
		t.Errorf("HealthCheckInterval=%v want 回落 %v", c.HealthCheckInterval, want)
	}
}

// enabled=false → interval 恒 0：调度器不启动循环（避免「关了还在跑」）。
func TestHealthCheckDisabledKeepsZero(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "c.json")
	body := `{"schedule":{"health_check_enabled":false,"health_check_minutes":30}}`
	if err := os.WriteFile(fp, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(fp)
	if err != nil {
		t.Fatal(err)
	}
	if c.HealthCheckInterval != 0 {
		t.Errorf("HealthCheckInterval=%v want 0（已禁用）", c.HealthCheckInterval)
	}
}
