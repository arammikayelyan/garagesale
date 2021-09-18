package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
)

// Respond marshals to a JSON and sends it to the client
func Respond(ctx context.Context, w http.ResponseWriter, val interface{}, statusCode int) error {

	v, ok := ctx.Value(KeyValues).(*Values)
	if !ok {
		return errors.New("web values missing from context")
	}
	v.StatusCode = statusCode

	if statusCode == http.StatusNoContent {
		w.WriteHeader(statusCode)
		return nil
	}

	data, err := json.Marshal(val)
	if err != nil {
		return errors.Wrap(err, "marshaling value to json")
	}

	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if _, err := w.Write(data); err != nil {
		return errors.Wrap(err, "writing to client")
	}

	return nil
}

// RespondError knows how to handle errors going to the client
func RespondError(ctx context.Context, w http.ResponseWriter, err error) error {

	// if the error was of the type *Error, the handler has
	// a specific status code an error to return.
	if webErr, ok := errors.Cause(err).(*Error); ok {
		er := ErrorResponse{
			Error:  webErr.Err.Error(),
			Fields: webErr.Fields,
		}

		if err := Respond(ctx, w, er, webErr.Status); err != nil {
			return err
		}
		return nil
	}

	er := ErrorResponse{
		Error: http.StatusText(http.StatusInternalServerError),
	}

	if err := Respond(ctx, w, er, http.StatusInternalServerError); err != nil {
		return err
	}
	return nil
}
