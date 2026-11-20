package console

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

func download(w http.ResponseWriter, contentType, name string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (c *Console) exportBackup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode       string `json:"mode"`
		Password   string `json:"password"`
		Passphrase string `json:"passphrase"`
		Confirm    bool   `json:"confirm"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Mode == "setup" {
		body, name, err := c.ExportSetup(r.Context())
		if err != nil {
			c.fail(w, err)
			return
		}
		download(w, "application/yaml", name, body)
		return
	}
	if !in.Confirm {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "confirm that the file holds every credential of this install", Code: "confirm_required"})
		return
	}
	if !c.verify(w, c.State.OwnerName(), in.Password) {
		return
	}
	body, name, err := c.ExportBackup(r.Context(), []byte(in.Passphrase))
	if err != nil {
		c.fail(w, err)
		return
	}
	download(w, "application/octet-stream", name, body)
}

func (c *Console) ownerOrFirstRun(next http.HandlerFunc) http.HandlerFunc {
	authed := c.authed(next)
	return func(w http.ResponseWriter, r *http.Request) {
		if c.State.HasOwner() {
			authed(w, r)
			return
		}
		next(w, r)
	}
}

func readUpload(w http.ResponseWriter, r *http.Request) ([]byte, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "send the file and its passphrase as a form")
		return nil, nil, false
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	f, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "send the file")
		return nil, nil, false
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		writeError(w, http.StatusBadRequest, "the file could not be read")
		return nil, nil, false
	}
	return raw, []byte(r.FormValue("passphrase")), true
}

func (c *Console) inspectUpload(w http.ResponseWriter, r *http.Request) {
	raw, passphrase, ok := readUpload(w, r)
	if !ok {
		return
	}
	s, err := c.InspectBackup(r.Context(), raw, passphrase)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (c *Console) importUpload(w http.ResponseWriter, r *http.Request) {
	raw, passphrase, ok := readUpload(w, r)
	if !ok {
		return
	}
	if !isBackup(raw) {
		if _, signedIn := c.Sessions.Get(r); !signedIn {
			writeError(w, http.StatusUnauthorized, "create the owner account first, then import a setup")
			return
		}
	}
	out, err := c.importFile(r.Context(), raw, passphrase)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (c *Console) importFile(ctx context.Context, raw, passphrase []byte) (any, error) {
	if isBackup(raw) {
		return c.RestoreBackup(ctx, raw, passphrase)
	}
	return c.ImportSetup(ctx, raw)
}

func splitHostBody(r *http.Request) ([]byte, []byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBackupBytes+maxHostSecret))
	if err != nil {
		return nil, nil, err
	}
	passphrase, file, _ := bytes.Cut(raw, []byte("\n"))
	return bytes.TrimRight(passphrase, "\r"), file, nil
}

func (c *Console) hostExport(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("mode") == "setup" {
		body, _, err := c.ExportSetup(r.Context())
		if err != nil {
			c.fail(w, err)
			return
		}
		_, _ = w.Write(body)
		return
	}
	passphrase, err := readSecret(r.Body)
	if err != nil {
		c.fail(w, err)
		return
	}
	body, _, err := c.ExportBackup(r.Context(), passphrase)
	if err != nil {
		c.fail(w, err)
		return
	}
	_, _ = w.Write(body)
}

func (c *Console) hostInspect(w http.ResponseWriter, r *http.Request) {
	passphrase, file, err := splitHostBody(r)
	var s BackupSummary
	if err == nil {
		s, err = c.InspectBackup(r.Context(), file, passphrase)
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (c *Console) hostImport(w http.ResponseWriter, r *http.Request) {
	passphrase, file, err := splitHostBody(r)
	var out any
	if err == nil {
		out, err = c.importFile(r.Context(), file, passphrase)
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
