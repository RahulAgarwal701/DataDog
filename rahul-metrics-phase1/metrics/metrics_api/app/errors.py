"""Contract error type: every non-2xx body is ErrorResponse {error:{code,message,details}}."""

STATUS_FOR_CODE = {
    "invalid_argument": 400,
    "not_found": 404,
    "upstream_unavailable": 503,
    "internal_error": 500,
}


class ApiError(Exception):
    def __init__(self, code: str, message: str, details: dict | None = None):
        super().__init__(message)
        self.code = code
        self.message = message
        self.details = details
        self.status = STATUS_FOR_CODE[code]

    def body(self) -> dict:
        err = {"code": self.code, "message": self.message}
        if self.details:
            err["details"] = self.details
        return {"error": err}
