package middleware

import "context"

type reqIDKey string

func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value("request_id").(string); ok {
		return reqID
	}
	if reqID, ok := ctx.Value(reqIDKey("request_id")).(string); ok {
		return reqID
	}
	return ""
}
