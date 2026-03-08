package orm

import (
	"context"
	"testing"
)

type testPlugin struct {
	name    string
	install func(host *PluginHost) error
}

func (p testPlugin) Name() string { return p.name }
func (p testPlugin) Install(host *PluginHost) error {
	if p.install != nil {
		return p.install(host)
	}
	return nil
}

func TestRegisterPluginAndDuplicate(t *testing.T) {
	ResetPlugins()
	t.Cleanup(ResetPlugins)

	called := 0
	err := RegisterPlugin(testPlugin{
		name: "audit",
		install: func(host *PluginHost) error {
			host.AddBeforeInterceptor(func(ctx context.Context, info OperationInfo) (context.Context, error) {
				called++
				return ctx, nil
			})
			return nil
		},
	})
	if err != nil {
		t.Fatalf("register plugin: %v", err)
	}

	_, _ = Query[crudUser](setupSQLiteDB(t)).WhereEq("id", int64(1)).All(context.Background())
	if called == 0 {
		t.Fatalf("expected plugin-installed interceptor to be called")
	}

	err = RegisterPlugin(testPlugin{name: "audit"})
	if err == nil || !HasCode(err, CodeConflict) {
		t.Fatalf("expected duplicate conflict error, got %v", err)
	}
}

func TestRegisterPluginValidation(t *testing.T) {
	ResetPlugins()
	t.Cleanup(ResetPlugins)

	if err := RegisterPlugin(nil); err == nil || !HasCode(err, CodeInvalidQuery) {
		t.Fatalf("expected invalid query for nil plugin, got %v", err)
	}
	if err := RegisterPlugin(testPlugin{}); err == nil || !HasCode(err, CodeInvalidQuery) {
		t.Fatalf("expected invalid query for empty plugin name, got %v", err)
	}
}
