package samsung

import "encoding/json"

const (
	EventD2DServiceMessage      = "d2d_service_message"
	EventD2DServiceMessageEvent = "d2d.service.message.event"
	EventChannelConnect         = "ms.channel.connect"
	EventChannelReady           = "ms.channel.ready"
)

const (
	endpointArtApp        = "com.samsung.art-app"
	endpointRemoteControl = "samsung.remote.control"
	portArtWSS            = 8002
)

// ArtContent is a single artwork item on the TV.
type ArtContent struct {
	ContentID  string `json:"content_id"`
	CategoryID string `json:"category_id"`
}

type connInfo struct {
	IP      string      `json:"ip"`
	Port    json.Number `json:"port"`
	Key     string      `json:"key"`
	Secured bool        `json:"secured"`
}

type artResponse struct {
	Event     string `json:"event"`
	RequestID string `json:"request_id"`
	ID        string `json:"id"`
	ErrorCode int    `json:"error_code,omitempty"`

	ContentListRaw json.RawMessage `json:"content_list,omitempty"`
	ContentID      string          `json:"content_id,omitempty"`
	Value          string          `json:"value,omitempty"`
	ConnInfoRaw    json.RawMessage `json:"conn_info,omitempty"`
}

func (a *artResponse) ContentList() string {
	return parsePolyString(a.ContentListRaw)
}

func (a *artResponse) ConnInfo() string {
	return parsePolyString(a.ConnInfoRaw)
}

func parsePolyString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

type wsResponse struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}
