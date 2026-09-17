package helper

import (
	"encoding/json"
	"errors"
	"fmt"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	hostreasoning "github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/gin-gonic/gin"
)

// ModelMappingQueueEntry is one upstream target in an ordered model-mapping
// queue. A channel mapping value may be a plain string (1:1 mapping) or a
// JSON array whose items are upstream model names or objects of the form
// {"model": "upstream", "retry": 2}; the queue is walked in JSON order.
type ModelMappingQueueEntry struct {
	UpstreamModel string
	Retry         int // extra attempts before moving to the next entry
}

// ResolveModelMappingQueue returns the ordered upstream queue for originModel
// from the channel mapping JSON, falling back to the reasoning base model
// name. A nil result means the mapping is 1:1 (or absent) for this model.
func ResolveModelMappingQueue(mappingJSON, originModel string) []ModelMappingQueueEntry {
	if mappingJSON == "" || mappingJSON == "{}" {
		return nil
	}
	var parsed map[string]json.RawMessage
	if err := rootcommon.Unmarshal([]byte(mappingJSON), &parsed); err != nil {
		return nil
	}
	candidates := []string{originModel}
	if base := hostreasoning.BaseModelName(originModel); base != originModel {
		candidates = append(candidates, base)
	}
	for _, candidate := range candidates {
		raw, exists := parsed[candidate]
		if !exists {
			continue
		}
		var items []any
		if err := rootcommon.Unmarshal(raw, &items); err != nil {
			continue // string value: 1:1 mapping handled by the chain logic
		}
		var queue []ModelMappingQueueEntry
		for _, item := range items {
			switch entry := item.(type) {
			case string:
				if entry != "" {
					queue = append(queue, ModelMappingQueueEntry{UpstreamModel: entry})
				}
			case map[string]any:
				model, _ := entry["model"].(string)
				retry := 0
				if r, ok := entry["retry"].(float64); ok && r > 0 {
					retry = int(r)
				}
				if model != "" {
					queue = append(queue, ModelMappingQueueEntry{UpstreamModel: model, Retry: retry})
				}
			}
		}
		if len(queue) > 0 {
			return queue
		}
	}
	return nil
}

// FirstHealthyMappingQueueEntry returns the first index at or after start whose
// entry is not in the per-(channel, upstream-model) error cooldown, or -1 when
// every remaining entry is currently cooled down. Callers decide the fail-open
// fallback: initial selection stays on index 0, advancing past a failed entry
// ends the queue walk.
func FirstHealthyMappingQueueEntry(channelId int, queue []ModelMappingQueueEntry, start int) int {
	for i := start; i < len(queue); i++ {
		if !service.ChannelInErrorCooldown(channelId, queue[i].UpstreamModel) {
			return i
		}
	}
	return -1
}

func ModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}

	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		modelMap := make(map[string]json.RawMessage)
		err := rootcommon.Unmarshal([]byte(modelMapping), &modelMap)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}

		// 支持链式模型重定向，最终使用链尾的模型。Values that are mapping
		// queues (JSON arrays) decode to no string here and are driven by the
		// per-attempt queue override instead.
		currentModel := info.OriginModelName
		visitedModels := map[string]bool{
			currentModel: true,
		}
		for {
			var mappedModel string
			if raw, exists := modelMap[currentModel]; exists {
				_ = rootcommon.Unmarshal(raw, &mappedModel)
			}
			baseModel := hostreasoning.BaseModelName(currentModel)
			if mappedModel == "" && baseModel != currentModel {
				if raw, exists := modelMap[baseModel]; exists {
					_ = rootcommon.Unmarshal(raw, &mappedModel)
				}
			}
			if mappedModel != "" {
				// 模型重定向循环检测，避免无限循环
				if visitedModels[mappedModel] {
					if mappedModel == currentModel {
						if currentModel == info.OriginModelName {
							info.IsModelMapped = false
							break
						}

						info.IsModelMapped = true
						break
					}
					return errors.New("model_mapping_contains_cycle")
				}
				visitedModels[mappedModel] = true
				currentModel = mappedModel
				info.IsModelMapped = true
			} else {
				break
			}
		}
		if info.IsModelMapped {
			info.UpstreamModelName = currentModel
		}
	}

	// An ordered mapping queue attempt pins the upstream model for this
	// attempt and takes precedence over the 1:1 chain resolution.
	if override := c.GetString(string(constant.ContextKeyChannelModelMappingQueue)); override != "" {
		info.UpstreamModelName = override
		info.IsModelMapped = true
	}

	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
