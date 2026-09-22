package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// DataCaptureSetting 控制是否把经过网关的 chat/completions 类会话（请求体+响应体）
// 落盘为 JSONL 文件，用于离线训练数据采集。采集的是用户会话明文，属敏感数据，默认关闭。
type DataCaptureSetting struct {
	// Enabled 总开关，关闭时中间件零开销直接放行。
	Enabled bool `json:"enabled"`
	// MaxBodyKB 单条请求/响应体缓存上限（KB），超出部分截断，防止超大 base64/长上下文撑爆磁盘与内存。
	MaxBodyKB int `json:"max_body_kb"`
	// Dir 输出目录，为空时回退到 <LogDir>/captures。
	Dir string `json:"dir"`
}

var dataCaptureSetting = DataCaptureSetting{
	Enabled:   false,
	MaxBodyKB: 512,
	Dir:       "",
}

func init() {
	config.GlobalConfig.Register("data_capture_setting", &dataCaptureSetting)
}

func GetDataCaptureSetting() *DataCaptureSetting {
	return &dataCaptureSetting
}

// MaxBodyBytes 返回单条 body 的字节上限，非正值时回退到默认 512KB。
func (s *DataCaptureSetting) MaxBodyBytes() int {
	kb := s.MaxBodyKB
	if kb <= 0 {
		kb = 512
	}
	return kb << 10
}
