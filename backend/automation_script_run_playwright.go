package backend

import (
	"context"
	"fmt"
	"strings"

	"ant-chrome/backend/internal/automation"
)

func automationSelectorStringValue(selector map[string]any, keys ...string) string {
	if selector == nil {
		return ""
	}

	for _, key := range keys {
		if value, ok := selector[key].(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func (a *App) automationSelectorProfileID(selector map[string]any) (string, error) {
	if profileID := automationSelectorStringValue(selector, "profileId"); profileID != "" {
		return profileID, nil
	}

	launchCode := automationSelectorStringValue(selector, "code", "launchCode")
	if launchCode == "" {
		return "", nil
	}
	if a.launchCodeSvc == nil {
		return "", fmt.Errorf("启动码服务未初始化，无法解析脚本目标实例")
	}
	profileID, err := a.launchCodeSvc.Resolve(launchCode)
	if err != nil {
		return "", fmt.Errorf("启动码 %q 未匹配到实例: %w", launchCode, err)
	}
	return strings.TrimSpace(profileID), nil
}

func normalizeAutomationRunSelector(selector map[string]any) map[string]any {
	launchCode := automationSelectorStringValue(selector, "launchCode")
	if launchCode == "" || automationSelectorStringValue(selector, "code") != "" {
		return selector
	}

	normalized := make(map[string]any, len(selector)+1)
	for key, value := range selector {
		normalized[key] = value
	}
	normalized["code"] = launchCode
	return normalized
}

func (a *App) ensurePlaywrightTargetReady(selector map[string]any) error {
	profileID, err := a.automationSelectorProfileID(selector)
	if err != nil {
		return fmt.Errorf("预启动脚本目标实例失败: %w", err)
	}
	if profileID == "" {
		return nil
	}

	if _, err := a.browserInstanceStartInternal(profileID, nil, nil, false, false, false, true, "", ""); err != nil {
		return fmt.Errorf("预启动脚本目标实例失败: %w", err)
	}
	return nil
}

func (a *App) runPlaywrightScript(ctx context.Context, script automation.ScriptRecord, input automation.ScriptRunRequest) (string, string, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if a.automationMgr == nil {
		return "", "脚本执行失败", "automation runtime manager is not initialized"
	}
	if a.config == nil || !a.config.Automation.Enabled {
		return "", "脚本执行失败", "自动化支持尚未启用"
	}
	if err := ctx.Err(); err != nil {
		return "", "脚本执行失败", automationRunContextErrorMessage(err)
	}
	if err := a.automationMgr.EnsureInstalled(ctx); err != nil {
		return "", "脚本执行失败", err.Error()
	}
	if err := ctx.Err(); err != nil {
		return "", "脚本执行失败", automationRunContextErrorMessage(err)
	}

	state := a.automationMgr.CurrentState()
	if !state.Ready {
		return "", "脚本执行失败", "自动化运行时尚未就绪"
	}

	paramsText := resolveAutomationRunJSONText(input.ParamsText, script.ParamsText, input.UseScriptParams)

	selector, targetSummary, err := a.resolveAutomationEffectiveSelector(script, input, false)
	if err != nil {
		return "", "脚本执行失败", err.Error()
	}
	selector = normalizeAutomationRunSelector(selector)
	if err := a.ensurePlaywrightTargetReady(selector); err != nil {
		return "", "脚本执行失败", err.Error()
	}
	if err := ctx.Err(); err != nil {
		return "", "脚本执行失败", automationRunContextErrorMessage(err)
	}
	params, err := parseAutomationJSONObject(paramsText, false)
	if err != nil {
		return "", "脚本执行失败", err.Error()
	}

	baseURL, authHeader, authValue, err := a.automationDemoEndpoint()
	if err != nil {
		return "", "脚本执行失败", err.Error()
	}

	scriptPath, artifactDir, cleanup, err := a.preparePlaywrightScriptWorkspace(state.RuntimeDir, script)
	if err != nil {
		return "", "脚本执行失败", err.Error()
	}
	defer cleanup()
	if err := ctx.Err(); err != nil {
		return "", "脚本执行失败", automationRunContextErrorMessage(err)
	}

	taskResult, err := a.automationMgr.RunScriptTask(ctx, automation.ScriptTaskRequest{
		TaskKey:          "script:" + script.ID,
		ScriptPath:       scriptPath,
		Selector:         selector,
		Params:           params,
		LaunchBaseURL:    baseURL,
		LaunchAuthHeader: authHeader,
		LaunchAuthValue:  authValue,
		ArtifactDir:      artifactDir,
		Timeout:          automationScriptRunTimeout(input),
	})
	if err != nil {
		return "", "脚本执行失败", err.Error()
	}
	if !taskResult.OK {
		errorText := strings.TrimSpace(taskResult.Error)
		if errorText == "" {
			errorText = "playwright script returned ok=false"
		}
		return taskResult.ResultText, appendAutomationRunSummary(taskResult.Summary, targetSummary), errorText
	}
	return taskResult.ResultText, appendAutomationRunSummary(taskResult.Summary, targetSummary), ""
}
