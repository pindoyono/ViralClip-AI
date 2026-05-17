package utils

import "github.com/gofiber/fiber/v2"

// Success sends a successful JSON response.
func Success(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

// Created sends a 201 Created JSON response.
func Created(c *fiber.Ctx, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    data,
	})
}

// SuccessMessage sends a successful response with a message.
func SuccessMessage(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": message,
	})
}

// BadRequest sends a 400 response.
func BadRequest(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "bad_request",
			"message": message,
		},
	})
}

// Unauthorized sends a 401 response.
func Unauthorized(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "unauthorized",
			"message": message,
		},
	})
}

// Forbidden sends a 403 response.
func Forbidden(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "forbidden",
			"message": message,
		},
	})
}

// NotFound sends a 404 response.
func NotFound(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "not_found",
			"message": message,
		},
	})
}

// InternalError sends a 500 response.
func InternalError(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "internal_error",
			"message": message,
		},
	})
}

// ValidationError sends a 422 response with field-level errors.
func ValidationError(c *fiber.Ctx, details map[string]string) error {
	return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "validation_error",
			"message": "Validation failed",
			"details": details,
		},
	})
}
