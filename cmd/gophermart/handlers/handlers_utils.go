package handlers

import (
	"io"
	"net/http"
)

type (
	responseData struct {
		status int
		size   int
	}

	loggingResponseWriter struct {
		http.ResponseWriter
		responseData *responseData
	}
)

func (r *loggingResponseWriter) Write(b []byte) (int, error) {
	size, err := r.ResponseWriter.Write(b)
	r.responseData.size += size
	return size, err
}

func (r *loggingResponseWriter) WriteHeader(statusCode int) {
	r.ResponseWriter.WriteHeader(statusCode)
	r.responseData.status = statusCode
}

type gzipWriter struct {
	http.ResponseWriter
	Writer io.Writer
}

func (w gzipWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (con *Controller) Debug(res http.ResponseWriter, formatString string, code int) {
	con.sugar.Debugf(formatString)
	if code != http.StatusOK {
		http.Error(res, formatString, code)
	} else {
		_, _ = res.Write([]byte(formatString + "\n"))
		res.WriteHeader(http.StatusOK)
	}
}
