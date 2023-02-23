package upload2dir

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
)

const (
	Version = "1.0"
)

func init() {
	caddy.RegisterModule(Upload2dir{})
	httpcaddyfile.RegisterHandlerDirective("upload2dir", parseCaddyfile)
}

// Middleware implements an HTTP handler that writes the
// uploaded file  to a file on the disk.
type Upload2dir struct {
	FileServerRoot string `json:"file_server_root"`
	DestField      string `json:"dest_field,omitempty"`
	FileFieldName  string `json:"file_field_name,omitempty"`
	MaxFilesize    int64  `json:"max_filesize_int,omitempty"`
	MaxFilesizeH   string `json:"max_filesize,omitempty"`
	MaxFormBuffer  int64  `json:"max_form_buffer_int,omitempty"`
	MaxFormBufferH string `json:"max_form_buffer,omitempty"`

	ctx    caddy.Context
	logger *zap.Logger
}

// CaddyModule returns the Caddy module information.
func (Upload2dir) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.upload2dir",
		New: func() caddy.Module { return new(Upload2dir) },
	}
}

// Provision implements caddy.Provisioner.
func (u *Upload2dir) Provision(ctx caddy.Context) error {
	u.ctx = ctx
	u.logger = ctx.Logger(u)

	repl := caddy.NewReplacer()

	if u.FileFieldName == "" {
		u.logger.Warn("Provision",
			zap.String("msg", "no FileFieldName specified (file_field_name), using the default one 'myFile'"),
		)
		u.FileFieldName = "file"
	}

	if u.MaxFilesize == 0 && u.MaxFilesizeH != "" {

		MaxFilesizeH := repl.ReplaceAll(u.MaxFilesizeH, "1GB")
		u.MaxFilesizeH = MaxFilesizeH

		size, err := humanize.ParseBytes(u.MaxFilesizeH)
		if err != nil {
			u.logger.Error("Provision ReplaceAll",
				zap.String("msg", "MaxFilesizeH: Error parsing max_filesize"),
				zap.String("MaxFilesizeH", u.MaxFilesizeH),
				zap.Error(err))
			return err
		}
		u.MaxFilesize = int64(size)
	} else {
		if u.MaxFilesize == 0 {
			size, err := humanize.ParseBytes("1GB")
			if err != nil {
				u.logger.Error("Provision int",
					zap.String("msg", "MaxFilesize: Error parsing max_filesize_int"),
					zap.Int64("MaxFilesize", u.MaxFilesize),
					zap.Error(err))
				return err
			}
			u.MaxFilesize = int64(size)
		}
	}

	if u.MaxFormBuffer == 0 && u.MaxFormBufferH != "" {

		MaxFormBufferH := repl.ReplaceAll(u.MaxFormBufferH, "1GB")
		u.MaxFormBufferH = MaxFormBufferH

		size, err := humanize.ParseBytes(u.MaxFormBufferH)
		if err != nil {
			u.logger.Error("Provision ReplaceAll",
				zap.String("msg", "MaxFormBufferH: Error parsing max_form_buffer"),
				zap.String("MaxFormBufferH", u.MaxFormBufferH),
				zap.Error(err))
			return err
		}
		u.MaxFormBuffer = int64(size)
	} else {
		if u.MaxFormBuffer == 0 {
			size, err := humanize.ParseBytes("1GB")
			if err != nil {
				u.logger.Error("Provision int",
					zap.String("msg", "MaxFormBufferH: Error parsing max_form_buffer_int"),
					zap.Int64("MaxFormBuffer", u.MaxFormBuffer),
					zap.Error(err))
				return err
			}
			u.MaxFormBuffer = int64(size)
		}
	}

	u.logger.Info("Current Config",
		zap.String("Version", Version),
		zap.String("dest_field", u.DestField),
		zap.Int64("max_filesize_int", u.MaxFilesize),
		zap.String("max_filesize", u.MaxFilesizeH),
		zap.Int64("max_form_buffer_int", u.MaxFormBuffer),
		zap.String("max_form_buffer", u.MaxFormBufferH),
	)

	return nil
}

// Validate implements caddy.Validator.
func (u *Upload2dir) Validate() error {
	// TODO: Do I need this func
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (u Upload2dir) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {

	if r.Method == http.MethodPut {
		return u.PutFile(w, r, next)
	}

	if r.Method == http.MethodDelete {
		return u.DeleteFile(w, r, next)
	}

	if r.Method == http.MethodPost {
		return u.CreateDir(w, r, next)
	}

	return next.ServeHTTP(w, r)
}

// CreateDir
//
//	@receiver u
//	@param w
//	@param r
//	@param next
//	@return error
func (u *Upload2dir) CreateDir(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	dest := r.FormValue(u.DestField)
	if len(dest) < 1 {
		dest = filepath.Join(u.FileServerRoot, r.URL.Path)
	}

	if dir, err := os.Stat(dest); err == nil && dir.IsDir() {
		w.Write(wrapFailureData(fmt.Sprintf("dir %s exists", dest), -1))
	} else {
		if err := os.MkdirAll(dest, os.ModePerm); err != nil {
			w.Write(wrapFailureData(fmt.Sprintf("mkdir %s error: %s", dest, err.Error()), -1))
		} else {
			w.Write(wrapSuccessData(nil))
		}
	}

	return next.ServeHTTP(w, r)

}

func (u *Upload2dir) DeleteFile(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	dest := r.FormValue(u.DestField)
	if len(dest) < 1 {
		dest = filepath.Join(u.FileServerRoot, r.URL.Path)
	}

	err := os.Remove(dest)
	w.WriteHeader(http.StatusOK)
	if err != nil {
		w.Write(wrapFailureData(fmt.Sprintf("delete %s error: %s", dest, err.Error()), -1))
	} else {
		w.Write(wrapSuccessData(nil))
	}
	return next.ServeHTTP(w, r)

}

// PutFile ServeHTTP
//
//	@receiver u
//	@param w
//	@param r
//	@param next
//	@return error
func (u *Upload2dir) PutFile(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {

	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	requuid, requuiderr := repl.GetString("http.request.uuid")
	if !requuiderr {
		requuid = "0"
		u.logger.Error("http.request.uuid",
			zap.Bool("requuiderr", requuiderr),
			zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))
	}

	repl.Set("http.upload.max_filesize", u.MaxFilesize)

	// FormFile returns the first file for the given file field key
	// it also returns the FileHeader so we can get the Filename,
	// the Header and the size of the file
	file, handler, ff_err := r.FormFile(u.FileFieldName)
	dest := r.FormValue(u.DestField)
	if len(dest) < 1 {
		dest = filepath.Join(u.FileServerRoot, r.URL.Path)
	}

	if ff_err != nil {
		u.logger.Error("FormFile Error",
			zap.String("requuid", requuid),
			zap.String("message", "Error Retrieving the File"),
			zap.Error(ff_err),
			zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))
		return caddyhttp.Error(http.StatusInternalServerError, ff_err)
	}
	defer file.Close()

	destDir, fileName := filepath.Split(dest)
	if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("mkdirall %s error: %s", destDir, err.Error()))
	}

	if target, err := os.Stat(dest); err == nil && target.Size() > 0 {
		if err := os.Rename(dest, filepath.Join(destDir, fmt.Sprintf("backup-%d.%s", time.Now().Unix(), fileName))); err != nil {
			u.logger.Error("FormFile Error",
				zap.String("requuid", requuid),
				zap.String("message", "Rename file error"),
				zap.Error(err),
				zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))
			return caddyhttp.Error(http.StatusInternalServerError, err)
		}
	}

	tempFile, tmpf_err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE, 0755)

	if tmpf_err != nil {
		u.logger.Error("TempFile Error",
			zap.String("requuid", requuid),
			zap.String("message", "Error at TempFile"),
			zap.Error(tmpf_err),
			zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))
		return caddyhttp.Error(http.StatusInternalServerError, tmpf_err)
	}
	defer tempFile.Close()

	// read all of the contents of our uploaded file into a
	// byte array
	//fileBytes, io_err := ioutil.ReadAll(file)
	fileBytes, io_err := io.Copy(tempFile, file)
	if io_err != nil {
		u.logger.Error("Copy Error",
			zap.String("requuid", requuid),
			zap.String("message", "Error at io.Copy"),
			zap.Error(io_err),
			zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))
		return caddyhttp.Error(http.StatusInternalServerError, io_err)
	}

	u.logger.Info("Successful Upload Info",
		zap.String("requuid", requuid),
		zap.String("Uploaded File", handler.Filename),
		zap.Int64("File Size", handler.Size),
		zap.Int64("written-bytes", fileBytes),
		zap.Any("MIME Header", handler.Header),
		zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))

	repl.Set("http.upload.filename", handler.Filename)
	repl.Set("http.upload.filesize", handler.Size)

	w.WriteHeader(http.StatusOK)
	w.Write(wrapSuccessData(map[string]interface{}{
		"dir": dest,
	}))
	return next.ServeHTTP(w, r)
}

func wrapSuccessData(data interface{}) []byte {
	bytes, _ := json.Marshal(map[string]interface{}{
		"code": 0,
		"data": data,
	})
	return bytes
}

func wrapFailureData(message string, code int64) []byte {
	bytes, _ := json.Marshal(map[string]interface{}{
		"code":    code,
		"message": message,
	})
	return bytes
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (u *Upload2dir) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {

	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {

			case "dest_field":
				if !d.Args(&u.DestField) {
					return d.ArgErr()
				}
			case "file_field_name":
				if !d.Args(&u.FileFieldName) {
					return d.ArgErr()
				}
			case "max_form_buffer":
				var sizeStr string
				if !d.AllArgs(&sizeStr) {
					return d.ArgErr()
				}
				size, err := humanize.ParseBytes(sizeStr)
				if err != nil {
					return d.Errf("parsing max_form_buffer: %v", err)
				}
				u.MaxFormBuffer = int64(size)
			case "max_form_buffer_int":
				var sizeStr string
				if !d.AllArgs(&sizeStr) {
					return d.ArgErr()
				}
				i, err := strconv.ParseInt(sizeStr, 10, 64)
				if err != nil {
					return d.Errf("parsing max_form_buffer_int: %v", err)
				}
				u.MaxFormBuffer = i
			case "max_filesize":
				var sizeStr string
				if !d.AllArgs(&sizeStr) {
					return d.ArgErr()
				}
				size, err := humanize.ParseBytes(sizeStr)
				if err != nil {
					return d.Errf("parsing max_filesize: %v", err)
				}
				u.MaxFilesize = int64(size)
			case "max_filesize_int":
				var sizeStr string
				if !d.AllArgs(&sizeStr) {
					return d.ArgErr()
				}
				i, err := strconv.ParseInt(sizeStr, 10, 64)
				if err != nil {
					return d.Errf("parsing max_filesize_int: %v", err)
				}
				u.MaxFilesize = i
			default:
				return d.Errf("unrecognized servers option '%s'", d.Val())
			}
		}
	}
	return nil
}

// parseCaddyfile parses the upload directive. It enables the upload
// of a file:
//
//	upload {
//	    dest_dir          <destination directory>
//	    max_filesize      <size>
//	    response_template [<path to a response template>]
//	}
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var u Upload2dir
	err := u.UnmarshalCaddyfile(h.Dispenser)
	return u, err
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Upload2dir)(nil)
	_ caddy.Validator             = (*Upload2dir)(nil)
	_ caddyhttp.MiddlewareHandler = (*Upload2dir)(nil)
	_ caddyfile.Unmarshaler       = (*Upload2dir)(nil)
)
