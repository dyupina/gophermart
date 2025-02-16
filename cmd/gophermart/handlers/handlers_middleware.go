package handlers

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (con *Controller) PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				con.sugar.Errorf("Error recovering from panic: %v", err)
				http.Error(res, "Error recovering from panic", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(res, req)
	})
}

func (con *Controller) LoggingMiddleware(next http.Handler) http.Handler {
	logFn := func(res http.ResponseWriter, req *http.Request) {
		sugar := con.sugar
		start := time.Now()
		uri := req.RequestURI
		method := req.Method

		if method == http.MethodGet {
			next.ServeHTTP(res, req)
			duration := time.Since(start)
			sugar.Infoln(
				"uri", uri,
				"method", method,
				"duration", duration,
			)
		}

		if method == http.MethodPost {
			responseData := &responseData{
				status: 0,
				size:   0,
			}
			lw := loggingResponseWriter{
				ResponseWriter: res,
				responseData:   responseData,
			}
			next.ServeHTTP(&lw, req)
			sugar.Infoln(
				"status", responseData.status,
				"size", responseData.size,
			)
		}

		if method == http.MethodDelete {
			responseData := &responseData{
				status: 0,
				size:   0,
			}
			lw := loggingResponseWriter{
				ResponseWriter: res,
				responseData:   responseData,
			}
			next.ServeHTTP(&lw, req)
			sugar.Infoln(
				"status", responseData.status,
				"size", responseData.size,
			)
		}
	}

	return http.HandlerFunc(logFn)
}

func (con *Controller) GzipDecodeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(req.Body)
			if err != nil {
				http.Error(res, "Bad Request: Unable to decode gzip body", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			req.Body = gz
		}
		next.ServeHTTP(res, req)
	})
}

func (con *Controller) GzipEncodeMiddleware(next http.Handler) http.Handler {
	compressFn := func(res http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(res, req)
			return
		}

		if !strings.Contains(req.Header.Get("Content-Type"), "application/json") &&
			!strings.Contains(req.Header.Get("Content-Type"), "text/html") {
			next.ServeHTTP(res, req)
			return
		}

		minSize := 1400
		contentLength, _ := strconv.Atoi(req.Header.Get("Content-Length"))
		if contentLength < minSize {
			next.ServeHTTP(res, req)
			return
		}

		gzip, err := gzip.NewWriterLevel(res, gzip.BestSpeed)
		if err != nil {
			http.Error(res, "Error creating gzip.Writer", http.StatusBadRequest)
			return
		}

		defer gzip.Close()

		res.Header().Set("Content-Encoding", "gzip")

		next.ServeHTTP(gzipWriter{ResponseWriter: res, Writer: gzip}, req)
	}
	return http.HandlerFunc(compressFn)
}

func (con *Controller) AuthenticateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		uidFromCookie, err := con.userService.GetUserIDFromCookie(req)

		if err != nil || uidFromCookie == "" {
			con.sugar.Debugf("(AuthenticateMiddleware) Missing or invalid cookie: %s", err)

			uid := uuid.New().String()
			if err := con.userService.SetUserIDCookie(res, uid); err != nil {
				con.sugar.Errorf("(AuthenticateMiddleware) Failed to set user ID cookie: %s", err.Error())
				http.Error(res, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// con.userService.InitUserURLs(uid)
			con.sugar.Debugf("(AuthenticateMiddleware) New user ID set in cookie: %s", uid)
			req.Header.Set("User-ID", uid)
		} else {
			con.sugar.Debugf("(AuthenticateMiddleware) Valid user ID from cookie: %s", uidFromCookie)
			req.Header.Set("User-ID", uidFromCookie)
		}

		next.ServeHTTP(res, req)
	})
}
