package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// ZerologMiddleware returns a Fiber middleware that logs each request with zerolog.
func ZerologMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		latency := time.Since(start)
		status := c.Response().StatusCode()

		event := log.Info()
		if status >= 500 {
			event = log.Error()
		} else if status >= 400 {
			event = log.Warn()
		}

		event.
			Str("request_id", c.Locals("requestid").(string)).
			Str("method", c.Method()).
			Str("path", c.Path()).
			Str("ip", c.IP()).
			Int("status", status).
			Dur("latency", latency).
			Str("user_agent", c.Get("User-Agent")).
			Msg("request")

		return err
	}
}

// ErrorHandler is the global Fiber error handler.
func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"
	errCode := "internal_error"

	var e *fiber.Error
	if ok := isError(err, &e); ok {
		code = e.Code
		message = e.Message
		switch code {
		case fiber.StatusNotFound:
			errCode = "not_found"
		case fiber.StatusBadRequest:
			errCode = "bad_request"
		case fiber.StatusUnauthorized:
			errCode = "unauthorized"
		case fiber.StatusForbidden:
			errCode = "forbidden"
		case fiber.StatusUnprocessableEntity:
			errCode = "validation_error"
		case fiber.StatusTooManyRequests:
			errCode = "rate_limit_exceeded"
		}
	}

	log.Error().Err(err).Int("status", code).Str("path", c.Path()).Msg("request error")

	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    errCode,
			"message": message,
		},
	})
}

func isError(err error, target **fiber.Error) bool {
	if fiberErr, ok := err.(*fiber.Error); ok {
		*target = fiberErr
		return true
	}
	return false
}
