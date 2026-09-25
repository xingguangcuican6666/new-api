package middleware

import (
	"io"
	"mime"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// maybeApplyZliuRouter 在渠道选择之前介入：当 zliu 路由启用且请求模型名等于触发用的
// 虚拟模型（默认 zliu-auto）时，调用本机路由器判定真实模型，改写 modelRequest.Model
// 与请求体的 "model" 字段。对其余模型零影响。
//
// 判定驱动力是 modelRequest.Model —— 它决定渠道选择、上游模型与计费（distributor 把它
// 写入 original_model 上下文键）。同时改写请求体是为了 pass-through 渠道也拿到真实模型，
// 并保持存储体自洽。整个过程 fail-open：任何失败都兜底成真实模型，绝不阻断请求。
//
// 放在 token「可用模型限制」校验之后调用，因此 ACL 校验的是用户实际请求的 zliu-auto，
// 而非改写后的具体模型——用户只需放行 zliu-auto，无须逐个放行快/强模型。
func maybeApplyZliuRouter(c *gin.Context, modelRequest *ModelRequest) {
	s := model_setting.GetZliuRouterSettings()
	if !s.Enabled || modelRequest.Model == "" || modelRequest.Model != s.TriggerModel {
		return
	}

	// 只有 JSON 请求体才承载可路由的对话；其余（表单/多段）无法安全改写，直接兜底。
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		modelRequest.Model = s.FailOpenModel()
		return
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		modelRequest.Model = s.FailOpenModel()
		return
	}
	raw, err := storage.Bytes()
	if err != nil {
		modelRequest.Model = s.FailOpenModel()
		return
	}

	// ResolveZliuRouteModel 永不报错：失败即返回兜底模型。
	target, _ := service.ResolveZliuRouteModel(c, raw)

	patched, err := sjson.SetBytes(raw, "model", target)
	if err != nil {
		// 改写请求体失败也不阻断：modelRequest.Model 仍走真实模型，转换路径据此下发。
		modelRequest.Model = target
		return
	}
	newStorage, err := common.CreateBodyStorage(patched)
	if err != nil {
		modelRequest.Model = target
		return
	}
	_ = storage.Close()
	c.Set(common.KeyBodyStorage, newStorage)
	c.Set(common.KeyRequestBody, nil)
	c.Request.Body = io.NopCloser(newStorage)
	c.Request.ContentLength = int64(len(patched))
	modelRequest.Model = target
	logger.LogDebug(c, "zliu_router: routed "+s.TriggerModel+" -> "+target)
}
