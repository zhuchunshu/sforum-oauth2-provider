package main

import (
	"context"
	"fmt"
	"strings"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// runLifecycle 覆盖 install/enable/upgrade/rollback/disable/uninstall 的
// plan/execute 步骤。市场插件没有外部副作用：enable 时确保 own_schema
// 表就绪，disable/uninstall 保留数据（retention: retain/export）。
func (s *oauthServer) runLifecycle(
	ctx context.Context,
	request *protocolwire.LifecycleRequest,
	progress *pluginv2.ProgressStream,
) error {
	if request == nil || progress == nil {
		return fmt.Errorf("missing lifecycle request")
	}
	stepID := strings.TrimSpace(request.GetStepId())
	if stepID == "" {
		stepID = lifecycleStepID(request.GetAction(), request.GetDryRun())
	}
	checkpoint := "plan"
	if !request.GetDryRun() {
		checkpoint = "execute"
	}

	// Host lifecycle 校验允许 PLANNED → RUNNING → SUCCEEDED。
	if err := progress.Send(&protocolwire.ProgressUpdate{
		StepId: stepID, State: protocolwire.ProgressState_PROGRESS_STATE_PLANNED,
		CompletedUnits: 0, TotalUnits: 3, Checkpoint: checkpoint + ".start",
	}); err != nil {
		return err
	}
	if err := progress.Send(&protocolwire.ProgressUpdate{
		StepId: stepID, State: protocolwire.ProgressState_PROGRESS_STATE_RUNNING,
		CompletedUnits: 1, TotalUnits: 3, Checkpoint: checkpoint + ".running",
	}); err != nil {
		return err
	}

	switch request.GetAction() {
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_INSTALL,
		protocolwire.LifecycleAction_LIFECYCLE_ACTION_ENABLE:
		if !request.GetDryRun() {
			// own_schema 表已在进程启动时 ensure；此处幂等重申。
			_ = s.store.ensureSchema(ctx)
		}
	}

	if err := progress.Send(&protocolwire.ProgressUpdate{
		StepId: stepID, State: protocolwire.ProgressState_PROGRESS_STATE_RUNNING,
		CompletedUnits: 2, TotalUnits: 3, Checkpoint: checkpoint + ".external",
	}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	default:
	}
	return progress.Send(&protocolwire.ProgressUpdate{
		StepId: stepID, State: protocolwire.ProgressState_PROGRESS_STATE_SUCCEEDED,
		CompletedUnits: 3, TotalUnits: 3, Checkpoint: checkpoint + ".done",
	})
}

func lifecycleActionName(action protocolwire.LifecycleAction) string {
	switch action {
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_INSTALL_PLAN:
		return "install.plan"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_INSTALL:
		return "install.execute"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_ENABLE:
		return "enable.execute"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_DISABLE:
		return "disable.execute"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_UPGRADE_PLAN:
		return "upgrade.plan"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_UPGRADE_BEFORE:
		return "upgrade.before"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_UPGRADE_AFTER:
		return "upgrade.after"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_ROLLBACK:
		return "rollback.execute"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_UNINSTALL_PLAN:
		return "uninstall.plan"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_UNINSTALL:
		return "uninstall.execute"
	case protocolwire.LifecycleAction_LIFECYCLE_ACTION_UNINSTALL_AFTER:
		return "uninstall.after"
	default:
		return "unknown"
	}
}

func lifecycleStepID(action protocolwire.LifecycleAction, dryRun bool) string {
	name := lifecycleActionName(action)
	if dryRun && !strings.HasSuffix(name, ".plan") {
		return name + ".plan"
	}
	return name
}
