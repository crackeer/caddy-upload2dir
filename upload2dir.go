package upload2dir

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

const (
	Version = "1.0"
)

// User
type User struct {
	Name   string `json:"name"`
	Action map[string]int
}

func init() {
	caddy.RegisterModule(Upload2dir{})
	httpcaddyfile.RegisterHandlerDirective("upload2dir", parseCaddyfile)
}

// Middleware implements an HTTP handler that writes the
// uploaded file  to a file on the disk.
type Upload2dir struct {
	FileServerRoot     string   `json:"file_server_root"`
	UserTokenCookieKey string   `json:"user_token_cookie_key"`
	UserConfig         []string `json:"user_config"`
	DestField          string   `json:"dest_field,omitempty"`
	FileFieldName      string   `json:"file_field_name,omitempty"`

	ctx    caddy.Context
	logger *zap.Logger
	users  map[string]*User
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

	if u.FileFieldName == "" {
		u.logger.Warn("Provision",
			zap.String("msg", "no FileFieldName specified (file_field_name), using the default one 'myFile'"),
		)
		u.FileFieldName = "file"
	}

	if u.UserTokenCookieKey == "" {
		u.logger.Warn("Provision",
			zap.String("msg", "no FileFieldName specified (file_field_name), using the default one 'myFile'"),
		)
		u.UserTokenCookieKey = "upload2dir-token"
	}

	if len(u.UserConfig) > 0 {
		u.logger.Error("Provision ReplaceAll",
			zap.String("msg", "user_config and"),
		)
		u.users = parseUserConfig(u.UserConfig)
	} else {
		u.users = map[string]*User{}
	}

	u.logger.Info("Current Config",
		zap.String("Version", Version),
		zap.String("dest_field", u.DestField),
	)

	return nil
}

// parseUserFile
//  @param filePath
//  @return map[string]User
//  @return error

// parseUserConfig token-aaabbcbad:username:delete_file/put_file/create_dir

// @param lines
// @return map[string]User
// @return error
func parseUserConfig(lines []string) map[string]*User {
	retData := map[string]*User{}
	for _, item := range lines {
		parts := strings.Split(item, ":")
		if len(parts) < 3 {
			continue
		}
		user := &User{
			Name:   parts[1],
			Action: map[string]int{},
		}
		for _, act := range strings.Split(parts[2], "/") {
			user.Action[strings.TrimSpace(act)] = 1
		}
		retData[parts[0]] = user

	}
	return retData
}

// Validate implements caddy.Validator.
func (u *Upload2dir) Validate() error {
	// TODO: Do I need this func
	return nil
}

// validUserAction
//
//	@receiver u
//	@param action
//	@return error
func (u Upload2dir) validUserAction(r *http.Request, action string) (*User, error) {
	accessDeny := errors.New("access deny")
	tokenCookie, err := r.Cookie(u.UserTokenCookieKey)

	if err != nil {
		u.logger.Error("GetUserTokenError", zap.String("error", err.Error()))
		return nil, accessDeny
	}
	u.logger.Info("GetUserToken", zap.String("token", tokenCookie.Value))
	token := tokenCookie.Value
	if len(token) < 1 {
		return nil, accessDeny
	}

	user, exist := u.users[strings.TrimSpace(token)]
	if !exist {
		return nil, accessDeny
	}
	u.logger.Info("GetUser", zap.String("User", user.Name))

	if _, ok := user.Action[action]; !ok {
		return user, accessDeny
	}

	return &User{
		Name: user.Name,
	}, nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (u Upload2dir) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	u.logger.Info("ServerHTTP", zap.String("Access", r.URL.Path), zap.String("method", r.Method))
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

	if user, err := u.validUserAction(r, "create_dir"); err != nil {
		return caddyhttp.Error(http.StatusForbidden, err)
	} else {
		u.logger.Info("action create_dir",
			zap.String("dest", dest), zap.String("user", user.Name))
	}

	if dir, err := os.Stat(dest); err == nil && dir.IsDir() {
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("dir %s exists", dest))
	} else {
		if err := os.MkdirAll(dest, os.ModePerm); err != nil {
			return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("mkdir %s error: %s", dest, err.Error()))
		}
	}

	return next.ServeHTTP(w, r)

}

func (u *Upload2dir) DeleteFile(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	dest := r.FormValue(u.DestField)
	if len(dest) < 1 {
		dest = filepath.Join(u.FileServerRoot, r.URL.Path)
	}

	if user, err := u.validUserAction(r, "delete_file"); err != nil {
		u.logger.Error("action delete_file",
			zap.String("file", dest), zap.String("error", err.Error()))
		return caddyhttp.Error(http.StatusForbidden, err)
	} else {
		u.logger.Info("action delete_file",
			zap.String("file", dest), zap.String("user", user.Name))
	}

	err := os.Remove(dest)
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("delete %s error: %s", dest, err.Error()))
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

	// FormFile returns the first file for the given file field key
	// it also returns the FileHeader so we can get the Filename,
	// the Header and the size of the file
	file, handler, ff_err := r.FormFile(u.FileFieldName)
	dest := r.FormValue(u.DestField)
	if len(dest) < 1 {
		dest = filepath.Join(u.FileServerRoot, r.URL.Path)
	}
	if user, err := u.validUserAction(r, "put_file"); err != nil {
		return caddyhttp.Error(http.StatusForbidden, err)
	} else {
		u.logger.Info("action delete_file",
			zap.String("file", dest), zap.String("user", user.Name))
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
	return next.ServeHTTP(w, r)
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
