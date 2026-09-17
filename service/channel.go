package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

func shouldCloseActiveWebSocketsAfterDisable(channelId int) bool {
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to check channel status before closing active websockets: channel_id=%d, error=%v", channelId, err))
		return true
	}
	return channel.Status != common.ChannelStatusEnabled
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		if shouldCloseActiveWebSocketsAfterDisable(channelError.ChannelId) {
			CloseActiveWebSocketsForChannel(channelError.ChannelId, ChannelDisabledCloseReason)
		}
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldRetryChannelError(err *types.NewAPIError, channelSettings *dto.ChannelOtherSettings) bool {
	if err == nil || channelSettings == nil || !channelSettings.AutomaticRetryOverrideEnabled {
		return false
	}
	codes := strings.TrimSpace(channelSettings.AutomaticRetryStatusCodes)
	keywords := strings.TrimSpace(channelSettings.AutomaticRetryKeywords)
	if codes == "" && keywords == "" {
		return false
	}
	if codes != "" {
		ranges, parseErr := operation_setting.ParseHTTPStatusCodeRanges(codes)
		if parseErr != nil || !operation_setting.ShouldDisableByStatusCodeRanges(ranges, err.StatusCode) {
			return false
		}
	}
	if keywords != "" {
		matched, _ := AcSearch(strings.ToLower(err.Error()), splitChannelKeywords(keywords), true)
		if !matched {
			return false
		}
	}
	return true
}

func splitChannelKeywords(value string) []string {
	keywords := make([]string, 0)
	for _, keyword := range strings.Split(value, "\n") {
		if keyword = strings.ToLower(strings.TrimSpace(keyword)); keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	return keywords
}

func matchesChannelDisableRule(err *types.NewAPIError, statusCodeRanges []operation_setting.StatusCodeRange, keywords []string) bool {
	if len(statusCodeRanges) == 0 && len(keywords) == 0 {
		return false
	}
	if len(statusCodeRanges) > 0 && !operation_setting.ShouldDisableByStatusCodeRanges(statusCodeRanges, err.StatusCode) {
		return false
	}
	if len(keywords) > 0 {
		matched, _ := AcSearch(strings.ToLower(err.Error()), keywords, true)
		if !matched {
			return false
		}
	}
	return true
}

func ShouldRuntimeDisableChannel(err *types.NewAPIError, channelSettings *dto.ChannelOtherSettings) bool {
	if !common.RuntimeAutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	statusCodeRanges := operation_setting.RuntimeAutomaticDisableStatusCodeRanges
	keywords := operation_setting.RuntimeAutomaticDisableKeywords
	hasChannelOverride := channelSettings != nil && channelSettings.RuntimeAutomaticDisableOverrideEnabled
	if channelSettings != nil && !hasChannelOverride && channelSettings.AutomaticDisableOverrideEnabled {
		hasChannelOverride = true
		channelSettings.RuntimeAutomaticDisableStatusCodes = channelSettings.AutomaticDisableStatusCodes
		channelSettings.RuntimeAutomaticDisableKeywords = channelSettings.AutomaticDisableKeywords
	}
	if hasChannelOverride {
		var parseErr error
		statusCodeRanges, parseErr = operation_setting.ParseHTTPStatusCodeRanges(channelSettings.RuntimeAutomaticDisableStatusCodes)
		if parseErr != nil {
			common.SysLog(fmt.Sprintf("invalid automatic disable status codes in channel override: %v", parseErr))
			return false
		}
		keywords = make([]string, 0)
		for _, keyword := range strings.Split(channelSettings.RuntimeAutomaticDisableKeywords, "\n") {
			keyword = strings.ToLower(strings.TrimSpace(keyword))
			if keyword != "" {
				keywords = append(keywords, keyword)
			}
		}
	}
	return matchesChannelDisableRule(err, statusCodeRanges, keywords)
}

func ShouldDisableChannel(err *types.NewAPIError, channelSettings *dto.ChannelOtherSettings) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	statusCodeRanges := operation_setting.AutomaticDisableStatusCodeRanges
	keywords := operation_setting.AutomaticDisableKeywords
	if channelSettings != nil {
		overrideEnabled := channelSettings.RuntimeAutomaticDisableOverrideEnabled
		statusCodes := channelSettings.RuntimeAutomaticDisableStatusCodes
		keywordText := channelSettings.RuntimeAutomaticDisableKeywords
		if !overrideEnabled && channelSettings.AutomaticDisableOverrideEnabled {
			overrideEnabled = true
			statusCodes = channelSettings.AutomaticDisableStatusCodes
			keywordText = channelSettings.AutomaticDisableKeywords
		}
		if overrideEnabled {
			var parseErr error
			statusCodeRanges, parseErr = operation_setting.ParseHTTPStatusCodeRanges(statusCodes)
			if parseErr != nil {
				common.SysLog(fmt.Sprintf("invalid automatic disable status codes in channel override: %v", parseErr))
				return false
			}
			keywords = splitChannelKeywords(keywordText)
		}
	}
	return matchesChannelDisableRule(err, statusCodeRanges, keywords)
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}
