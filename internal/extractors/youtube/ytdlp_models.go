package youtube

type YtDlpResponse struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Duration     float64        `json:"duration"`
	Uploader     string         `json:"uploader"`
	Thumbnail    string         `json:"thumbnail"`
	Availability string         `json:"availability"`
	LiveStatus   string         `json:"live_status"`
	IsLive       bool           `json:"is_live"`
	Formats      []*YtDlpFormat `json:"formats"`
}

type YtDlpFormat struct {
	FormatID       string  `json:"format_id"`
	URL            string  `json:"url"`
	Ext            string  `json:"ext"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	FileSize       int64   `json:"filesize"`
	FileSizeApprox int64   `json:"filesize_approx"`
	TBR            float64 `json:"tbr"`
	VBR            float64 `json:"vbr"`
	ABR            float64 `json:"abr"`
	Protocol       string  `json:"protocol"`
	FormatNote     string  `json:"format_note"`
	Language       string  `json:"language"`
}
