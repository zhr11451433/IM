package errs

const (
	CodeSuccess              = 0
	CodeAppKeyRequired       = 10001
	CodeNonceRequired        = 10002
	CodeTimestampRequired    = 10003
	CodeSignatureRequired    = 10004
	CodeAppNotExisted        = 10005
	CodeSignatureFail        = 10006
	CodeReqBodyIllegal       = 10007
	CodeInternalTimeout      = 10008
	CodeParamRequired        = 10010
	CodeUserNotExist         = 10102
	CodeConnectAppNotExist   = 11003
	CodeConnectTokenIllegal  = 11004
	CodeConnectTokenAuthFail = 11005
	CodeConnectTokenExpired  = 11006
	CodeConnectParamIllegal  = 11015
)

type APIError struct {
	HTTPCode int
	Msg      string `json:"msg"`
	Code     int    `json:"code"`
}

func AppKeyRequired() APIError {
	return APIError{HTTPCode: 400, Code: CodeAppKeyRequired, Msg: "appkey is required"}
}
func NonceRequired() APIError {
	return APIError{HTTPCode: 400, Code: CodeNonceRequired, Msg: "nonce is required"}
}
func TimestampRequired() APIError {
	return APIError{HTTPCode: 400, Code: CodeTimestampRequired, Msg: "timestamp is required"}
}
func SignatureRequired() APIError {
	return APIError{HTTPCode: 400, Code: CodeSignatureRequired, Msg: "signature is required"}
}
func AppNotExisted() APIError {
	return APIError{HTTPCode: 500, Code: CodeAppNotExisted, Msg: "app not existed"}
}
func SignatureFail() APIError {
	return APIError{HTTPCode: 403, Code: CodeSignatureFail, Msg: "signature is fail"}
}
func ReqBodyIllegal() APIError {
	return APIError{HTTPCode: 400, Code: CodeReqBodyIllegal, Msg: "request body is illegal"}
}
func InternalTimeout() APIError {
	return APIError{HTTPCode: 500, Code: CodeInternalTimeout, Msg: "internal service timeout"}
}
func ParamRequired() APIError {
	return APIError{HTTPCode: 400, Code: CodeParamRequired, Msg: "required params missing."}
}
func UserNotExist() APIError {
	return APIError{HTTPCode: 200, Code: CodeUserNotExist, Msg: "user not exist"}
}
