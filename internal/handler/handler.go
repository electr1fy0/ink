package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/electr1fy0/ink/internal/app"
	"github.com/electr1fy0/ink/internal/store"
)

type Handler struct {
	App *app.App
}

type HttpError struct {
	Status  int
	Message string
	Err     error
}

func (h *HttpError) Error() string {
	return h.Message
}

func HttpErrorResponse(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	var httpErr *HttpError
	if ok := errors.As(err, &httpErr); ok {
		http.Error(w, httpErr.Message, httpErr.Status)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func Handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		HttpErrorResponse(w, fn(w, r))
	}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	value, ok := h.App.Get(key)
	if !ok {
		return &HttpError{
			Status:  http.StatusNotFound,
			Message: "not found",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"key":   key,
		"value": value.Value,
	}); err != nil {
		return &HttpError{
			Status:  http.StatusInternalServerError,
			Message: "failed to encode response",
			Err:     err,
		}
	}

	return nil
}

func (h *Handler) InternalPut(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	var toWrite store.Entry
	if err := json.NewDecoder(r.Body).Decode(&toWrite); err != nil {
		return &HttpError{
			Status:  http.StatusBadRequest,
			Message: "failed to decode internal put body",
			Err:     err,
		}
	}

	if err := h.App.InternalPut(key, toWrite); err != nil {
		return &HttpError{
			Status:  http.StatusInternalServerError,
			Message: err.Error(),
			Err:     err,
		}
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("written"))

	return nil
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	var v struct {
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return &HttpError{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
			Err:     err,
		}
	}

	if err := h.App.Put(key, v.Value); err != nil {
		// Distinguish between quorum failure and other errors
		status := http.StatusInternalServerError
		if err.Error() == "failed to reach quorum of 2 acks" {
			status = http.StatusServiceUnavailable
		}
		return &HttpError{
			Status:  status,
			Message: err.Error(),
			Err:     err,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"response": "quorum achieved",
	}); err != nil {
		return &HttpError{
			Status:  http.StatusInternalServerError,
			Message: "failed to encode response",
			Err:     err,
		}
	}

	return nil
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	if err := h.App.Delete(key); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "key not found" {
			status = http.StatusNotFound
		}
		return &HttpError{
			Status:  status,
			Message: err.Error(),
			Err:     err,
		}
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(h.App.GetAll()); err != nil {
		return &HttpError{
			Status:  http.StatusInternalServerError,
			Message: "failed to encode response",
			Err:     err,
		}
	}
	return nil
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("don't worry about me mate"))
}
