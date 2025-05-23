package main

import "errors"

// ErrNotFound is returned when an item is not found in the store.
var ErrNotFound = errors.New("item not found")

// ErrInvalidInput is returned when the input payload is invalid.
var ErrInvalidInput = errors.New("invalid input")
