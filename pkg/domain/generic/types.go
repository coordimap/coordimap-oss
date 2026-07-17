package generic

type ResponseStatus struct {
	HTTPCode  int    `json:"http_code"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code,omitempty"`
}
