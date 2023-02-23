package upload2dir

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
)

const (
	Version = "0.9"
)

func init() {
	caddy.RegisterModule(Upload2dir{})
	httpcaddyfile.RegisterHandlerDirective("upload2dir", parseCaddyfile)
}

// Middleware implements an HTTP handler that writes the
// uploaded file  to a file on the disk.
type Upload2dir struct {
	DestDirField     string `json:"dest_dir_field,omitempty"`
	FileFieldName    string `json:"file_field_name,omitempty"`
	MaxFilesize      int64  `json:"max_filesize_int,omitempty"`
	MaxFilesizeH     string `json:"max_filesize,omitempty"`
	MaxFormBuffer    int64  `json:"max_form_buffer_int,omitempty"`
	MaxFormBufferH   string `json:"max_form_buffer,omitempty"`
	ResponseTemplate string `json:"response_template,omitempty"`

	MyTlsSetting struct {
		InsecureSkipVerify bool   `json:"insecure,omitempty"`
		CAPath             string `json:"capath,omitempty"`
	}

	// TODO: Handle notify Body

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

	if u.DestDirField == "" {
		u.logger.Error("Provision",
			zap.String("msg", "no Destination Directory specified (dest_dir)"))
		return fmt.Errorf("no Destination Directory specified (dest_dir)")
	}

	if u.FileFieldName == "" {
		u.logger.Warn("Provision",
			zap.String("msg", "no FileFieldName specified (file_field_name), using the default one 'myFile'"),
		)
		u.FileFieldName = "myFile"
	}

	if u.ResponseTemplate == "" {
		u.logger.Warn("Provision",
			zap.String("msg", "no ResponseTemplate specified (response_template), using the default one"),
		)
		u.ResponseTemplate = "upload-resp-template.txt"
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
		zap.String("dest_dir_field", u.DestDirField),
		zap.Int64("max_filesize_int", u.MaxFilesize),
		zap.String("max_filesize", u.MaxFilesizeH),
		zap.Int64("max_form_buffer_int", u.MaxFormBuffer),
		zap.String("max_form_buffer", u.MaxFormBufferH),
		zap.String("response_template", u.ResponseTemplate),
		zap.String("capath", u.MyTlsSetting.CAPath),
		zap.Bool("insecure", u.MyTlsSetting.InsecureSkipVerify),
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

	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

	requuid, requuiderr := repl.GetString("http.request.uuid")
	if !requuiderr {
		requuid = "0"
		u.logger.Error("http.request.uuid",
			zap.Bool("requuiderr", requuiderr),
			zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))
	}

	repl.Set("http.upload.max_filesize", u.MaxFilesize)

	r.Body = http.MaxBytesReader(w, r.Body, u.MaxFilesize)
	if max_size_err := r.ParseMultipartForm(u.MaxFormBuffer); max_size_err != nil {
		u.logger.Error("ServeHTTP",
			zap.String("requuid", requuid),
			zap.String("message", "The uploaded file is too big. Please choose an file that's less than MaxFilesize."),
			zap.String("MaxFilesize", humanize.Bytes(uint64(u.MaxFilesize))),
			zap.Error(max_size_err),
			zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))
		return caddyhttp.Error(http.StatusRequestEntityTooLarge, max_size_err)
	}

	// FormFile returns the first file for the given file field key
	// it also returns the FileHeader so we can get the Filename,
	// the Header and the size of the file
	file, handler, ff_err := r.FormFile(u.FileFieldName)
	destDir := r.FormValue(u.DestDirField)
	if len(destDir) < 1 {
		u.logger.Error("FormValue Error",
			zap.String("requuid", requuid),
			zap.String("message", "Error Retrieving the File"),
			zap.Error(ff_err),
			zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("form field %s required", destDir))
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

	// Create the file within the DestDir directory
	if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("mkdirall %s error: %s", destDir, err.Error()))
	}

	tempFile, tmpf_err := os.OpenFile(filepath.Join(destDir, handler.Filename), os.O_RDWR|os.O_CREATE, 0755)

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
	// write this byte array to our temporary file
	//tempFile.Write(fileBytes)

	u.logger.Info("Successful Upload Info",
		zap.String("requuid", requuid),
		zap.String("Uploaded File", handler.Filename),
		zap.Int64("File Size", handler.Size),
		zap.Int64("written-bytes", fileBytes),
		zap.Any("MIME Header", handler.Header),
		zap.Object("request", caddyhttp.LoggableHTTPRequest{Request: r}))

	repl.Set("http.upload.filename", handler.Filename)
	repl.Set("http.upload.filesize", handler.Size)

	if u.ResponseTemplate != "" {
		r.URL.Path = "/" + u.ResponseTemplate
	}

	return next.ServeHTTP(w, r)
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (u *Upload2dir) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {

	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {

			case "dest_dir_field":
				if !d.Args(&u.DestDirField) {
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
			case "response_template":
				if !d.Args(&u.ResponseTemplate) {
					return d.ArgErr()
				}
			case "insecure":
				if !d.NextArg() {
					return d.ArgErr()
				}
				u.MyTlsSetting.InsecureSkipVerify = true
			case "capath":
				if !d.Args(&u.MyTlsSetting.CAPath) {
					return d.ArgErr()
				}
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
