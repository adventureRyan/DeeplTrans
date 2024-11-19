package domain

type TranslateRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"` // 源语言
	TargetLang string `json:"target_lang"` // 目标语言
}

type TranslateResponse struct {
	Code int    `json:"code"` // 响应代码
	Data string `json:"data"`
}

type YingTuResponse struct {
	Code int `json:"code"`
	Data struct {
		Total        int                 `json:"total"`
		Arr          []YingTuResponseArr `json:"arr"`
		ConsumeQuota string              `json:"consume_quota"`
		RestQuota    string              `json:"rest_quota"`
	}
}

type YingTuResponseArr struct {
	Url string `json:"url"`
}
