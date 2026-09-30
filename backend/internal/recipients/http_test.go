package recipients

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// formField is one multipart field; a field named "file" is sent as a file part.
type formField struct {
	name    string
	content []byte
}

// multipartBody builds a multipart/form-data body with fields in order.
func multipartBody(t *testing.T, fields ...formField) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range fields {
		var w io.Writer
		var err error
		if f.name == "file" {
			w, err = mw.CreateFormFile(f.name, "odbiorcy.csv")
		} else {
			w, err = mw.CreateFormField(f.name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(f.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func checkImport(t *testing.T, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/recipients/import/check", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	HandleCheckImport(slog.New(slog.NewTextHandler(io.Discard, nil)))(rec, req)
	return rec
}

func TestHandleCheckImportReport(t *testing.T) {
	file := "first_name,last_name,email,phone,type\n" +
		"Jan,Kowalski,jan.kowalski@example.test,,parent\n" +
		"Lena,,lena.nowak@example.test,,student\n" +
		"Janek,Kowalski,JAN.KOWALSKI@example.test,,parent\n"
	body, ct := multipartBody(t, formField{"note", []byte("ignored")}, formField{"file", []byte(file)})

	rec := checkImport(t, body, ct)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	want := `{
		"valid": [{"row": 2, "recipient": {"firstName": "Jan", "lastName": "Kowalski", "email": "jan.kowalski@example.test", "phone": null, "type": "parent"}}],
		"invalid": [{"row": 3, "errors": [{"field": "last_name", "message": "last name is required"}]}],
		"duplicates": [{"row": 4, "field": "email", "duplicateOfRow": 2}]
	}`
	assertJSONEqual(t, rec.Body.Bytes(), want)
}

func TestHandleCheckImportEmptyListsAreArrays(t *testing.T) {
	body, ct := multipartBody(t, formField{"file", []byte("first_name,last_name,email,phone,type\n")})

	rec := checkImport(t, body, ct)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	assertJSONEqual(t, rec.Body.Bytes(), `{"valid": [], "invalid": [], "duplicates": []}`)
}

func TestHandleCheckImportFileErrors(t *testing.T) {
	multipartWith := func(fields ...formField) func(t *testing.T) (io.Reader, string) {
		return func(t *testing.T) (io.Reader, string) { return multipartBody(t, fields...) }
	}
	file := func(content string) formField { return formField{"file", []byte(content)} }
	cases := []struct {
		name       string
		request    func(t *testing.T) (io.Reader, string)
		wantStatus int
		wantCode   string
	}{
		{"not multipart", func(*testing.T) (io.Reader, string) {
			return strings.NewReader(`{"file": "x"}`), "application/json"
		}, http.StatusBadRequest, "invalid_request"},
		{"no file field", multipartWith(formField{"note", []byte("x")}), http.StatusBadRequest, "missing_file"},
		{"empty file", multipartWith(file("")), http.StatusUnprocessableEntity, "empty_file"},
		{"missing columns", multipartWith(file("imie,nazwisko\n")), http.StatusUnprocessableEntity, "missing_columns"},
		{"invalid header", multipartWith(file("imie,first_name,last_name,email,phone,type\n")), http.StatusUnprocessableEntity, "invalid_header"},
		{"legacy xls", multipartWith(file("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1")), http.StatusUnprocessableEntity, "unsupported_format"},
		{"unclosed quote", multipartWith(file("first_name,last_name,email,phone,type\n\"Jan,Kowalski,,,parent\nMaria,Kowalska,,,parent\n")), http.StatusUnprocessableEntity, "unclosed_quote"},
		{"too many rows", multipartWith(file("first_name,last_name,email,phone,type\n" + strings.Repeat("Jan,Kowalski,,+48500100101,parent\n", MaxImportRows+1))), http.StatusUnprocessableEntity, "too_many_rows"},
		{"file too large", multipartWith(file(strings.Repeat("a", MaxImportFileSize+1))), http.StatusRequestEntityTooLarge, "file_too_large"},
		// A huge field before the file trips the request cap, not the file limit.
		{"body over the request cap", multipartWith(formField{"note", bytes.Repeat([]byte("a"), maxImportBody)}, file("x")), http.StatusRequestEntityTooLarge, "file_too_large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, ct := tc.request(t)
			rec := checkImport(t, body, ct)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body)
			}
			var got fileErrorJSON
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body %q: %v", rec.Body, err)
			}
			if got.Code != tc.wantCode || got.Message == "" {
				t.Errorf("body = %+v, want code %q with a message", got, tc.wantCode)
			}
		})
	}
}

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("decode response %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want: %v", err)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if !bytes.Equal(gb, wb) {
		t.Errorf("response =\n%s\nwant\n%s", gb, wb)
	}
}
