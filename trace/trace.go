package trace

var HmsTraceKey = struct{}{}

type HmsTrace struct {
	GotRequestBody    func([]byte)
	GotResponseBody   func([]byte)
	GotResponseStatus func(int)
}
