package lua

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// loaderRevoco is the module loader for the "revoco" module.
func (r *Runtime) loaderRevoco(L *lua.LState) int {
	// Create the module table
	mod := L.NewTable()

	// Register submodules
	registerFileModule(L, mod)
	registerTimeModule(L, mod)
	registerJSONModule(L, mod)
	registerHashModule(L, mod)
	registerLogModule(L, mod)
	registerExecModule(L, mod)
	registerHTTPModule(L, mod)

	L.Push(mod)
	return 1
}

// ══════════════════════════════════════════════════════════════════════════════
// File Module (revoco.file.* and revoco.*)
// ══════════════════════════════════════════════════════════════════════════════

func registerFileModule(L *lua.LState, mod *lua.LTable) {
	// Top-level convenience functions
	mod.RawSetString("glob", L.NewFunction(luaGlob))
	mod.RawSetString("readFile", L.NewFunction(luaReadFile))
	mod.RawSetString("readJSON", L.NewFunction(luaReadJSON))
	mod.RawSetString("writeFile", L.NewFunction(luaWriteFile))
	mod.RawSetString("exists", L.NewFunction(luaExists))
	mod.RawSetString("isDir", L.NewFunction(luaIsDir))
	mod.RawSetString("mkdir", L.NewFunction(luaMkdir))
	mod.RawSetString("copy", L.NewFunction(luaCopy))
	mod.RawSetString("move", L.NewFunction(luaMove))
	mod.RawSetString("remove", L.NewFunction(luaRemove))
	mod.RawSetString("tempfile", L.NewFunction(luaTempfile))

	// Path functions
	mod.RawSetString("join", L.NewFunction(luaPathJoin))
	mod.RawSetString("dirname", L.NewFunction(luaDirname))
	mod.RawSetString("basename", L.NewFunction(luaBasename))
	mod.RawSetString("extname", L.NewFunction(luaExtname))
	mod.RawSetString("abs", L.NewFunction(luaAbs))
}

func luaGlob(L *lua.LState) int {
	base := L.CheckString(1)
	pattern := L.CheckString(2)

	// Construct full pattern
	fullPattern := filepath.Join(base, pattern)

	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Return as table
	tbl := L.NewTable()
	for i, m := range matches {
		tbl.RawSetInt(i+1, lua.LString(m))
	}

	L.Push(tbl)
	return 1
}

func luaReadFile(L *lua.LState) int {
	path := L.CheckString(1)

	content, err := os.ReadFile(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LString(content))
	return 1
}

func luaReadJSON(L *lua.LState) int {
	path := L.CheckString(1)

	content, err := os.ReadFile(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	var data any
	if err := json.Unmarshal(content, &data); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(goValueToLua(L, data))
	return 1
}

func luaWriteFile(L *lua.LState) int {
	path := L.CheckString(1)
	content := L.CheckString(2)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	return 1
}

func luaExists(L *lua.LState) int {
	path := L.CheckString(1)
	_, err := os.Stat(path)
	L.Push(lua.LBool(err == nil))
	return 1
}

func luaIsDir(L *lua.LState) int {
	path := L.CheckString(1)
	info, err := os.Stat(path)
	if err != nil {
		L.Push(lua.LFalse)
		return 1
	}
	L.Push(lua.LBool(info.IsDir()))
	return 1
}

func luaMkdir(L *lua.LState) int {
	path := L.CheckString(1)

	if err := os.MkdirAll(path, 0755); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	return 1
}

func luaCopy(L *lua.LState) int {
	src := L.CheckString(1)
	dst := L.CheckString(2)

	// Open source
	srcFile, err := os.Open(src)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer srcFile.Close()

	// Create destination
	dstFile, err := os.Create(dst)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer dstFile.Close()

	// Copy
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	return 1
}

func luaMove(L *lua.LState) int {
	src := L.CheckString(1)
	dst := L.CheckString(2)

	if err := os.Rename(src, dst); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	return 1
}

func luaRemove(L *lua.LState) int {
	path := L.CheckString(1)

	if err := os.RemoveAll(path); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	return 1
}

func luaTempfile(L *lua.LState) int {
	name := L.OptString(1, "temp")

	f, err := os.CreateTemp("", name+"_*")
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	path := f.Name()
	f.Close()

	L.Push(lua.LString(path))
	return 1
}

func luaPathJoin(L *lua.LState) int {
	n := L.GetTop()
	parts := make([]string, n)
	for i := 1; i <= n; i++ {
		parts[i-1] = L.CheckString(i)
	}
	L.Push(lua.LString(filepath.Join(parts...)))
	return 1
}

func luaDirname(L *lua.LState) int {
	path := L.CheckString(1)
	L.Push(lua.LString(filepath.Dir(path)))
	return 1
}

func luaBasename(L *lua.LState) int {
	path := L.CheckString(1)
	L.Push(lua.LString(filepath.Base(path)))
	return 1
}

func luaExtname(L *lua.LState) int {
	path := L.CheckString(1)
	L.Push(lua.LString(filepath.Ext(path)))
	return 1
}

func luaAbs(L *lua.LState) int {
	path := L.CheckString(1)
	abs, err := filepath.Abs(path)
	if err != nil {
		L.Push(lua.LString(path))
		return 1
	}
	L.Push(lua.LString(abs))
	return 1
}

// ══════════════════════════════════════════════════════════════════════════════
// Time Module (revoco.time.* and revoco.*)
// ══════════════════════════════════════════════════════════════════════════════

func registerTimeModule(L *lua.LState, mod *lua.LTable) {
	mod.RawSetString("now", L.NewFunction(luaNow))
	mod.RawSetString("parseTime", L.NewFunction(luaParseTime))
	mod.RawSetString("formatTime", L.NewFunction(luaFormatTime))

	// Also create time submodule
	timeMod := L.NewTable()
	timeMod.RawSetString("now", L.NewFunction(luaNow))
	timeMod.RawSetString("parse", L.NewFunction(luaParseTime))
	timeMod.RawSetString("format", L.NewFunction(luaFormatTime))
	mod.RawSetString("time", timeMod)
}

func luaNow(L *lua.LState) int {
	L.Push(lua.LNumber(time.Now().Unix()))
	return 1
}

func luaParseTime(L *lua.LState) int {
	str := L.CheckString(1)
	format := L.OptString(2, "")

	var t time.Time
	var err error

	if format != "" {
		// Use Go time format
		t, err = time.Parse(format, str)
	} else {
		// Try common formats
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006:01:02 15:04:05",
			"2006-01-02",
			"Jan 2, 2006",
			"January 2, 2006",
		}

		for _, f := range formats {
			t, err = time.Parse(f, str)
			if err == nil {
				break
			}
		}
	}

	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Return as table with components
	tbl := L.NewTable()
	tbl.RawSetString("unix", lua.LNumber(t.Unix()))
	tbl.RawSetString("year", lua.LNumber(t.Year()))
	tbl.RawSetString("month", lua.LNumber(int(t.Month())))
	tbl.RawSetString("day", lua.LNumber(t.Day()))
	tbl.RawSetString("hour", lua.LNumber(t.Hour()))
	tbl.RawSetString("minute", lua.LNumber(t.Minute()))
	tbl.RawSetString("second", lua.LNumber(t.Second()))
	tbl.RawSetString("iso", lua.LString(t.Format(time.RFC3339)))

	L.Push(tbl)
	return 1
}

func luaFormatTime(L *lua.LState) int {
	// Accept either unix timestamp or time table
	var t time.Time

	arg := L.Get(1)
	switch v := arg.(type) {
	case lua.LNumber:
		t = time.Unix(int64(v), 0)
	case *lua.LTable:
		unix := v.RawGetString("unix")
		if n, ok := unix.(lua.LNumber); ok {
			t = time.Unix(int64(n), 0)
		} else {
			L.Push(lua.LNil)
			L.Push(lua.LString("invalid time table"))
			return 2
		}
	default:
		L.Push(lua.LNil)
		L.Push(lua.LString("expected number or table"))
		return 2
	}

	format := L.OptString(2, time.RFC3339)
	L.Push(lua.LString(t.Format(format)))
	return 1
}

// ══════════════════════════════════════════════════════════════════════════════
// JSON Module (revoco.json.*)
// ══════════════════════════════════════════════════════════════════════════════

func registerJSONModule(L *lua.LState, mod *lua.LTable) {
	jsonMod := L.NewTable()
	jsonMod.RawSetString("encode", L.NewFunction(luaJSONEncode))
	jsonMod.RawSetString("decode", L.NewFunction(luaJSONDecode))
	mod.RawSetString("json", jsonMod)
}

func luaJSONEncode(L *lua.LState) int {
	val := L.Get(1)
	goVal := luaValueToGo(val)

	data, err := json.Marshal(goVal)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LString(data))
	return 1
}

func luaJSONDecode(L *lua.LState) int {
	str := L.CheckString(1)

	var data any
	if err := json.Unmarshal([]byte(str), &data); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(goValueToLua(L, data))
	return 1
}

// ══════════════════════════════════════════════════════════════════════════════
// Hash Module (revoco.hash.*)
// ══════════════════════════════════════════════════════════════════════════════

func registerHashModule(L *lua.LState, mod *lua.LTable) {
	hashMod := L.NewTable()
	hashMod.RawSetString("md5", L.NewFunction(luaHashMD5))
	hashMod.RawSetString("sha256", L.NewFunction(luaHashSHA256))
	hashMod.RawSetString("file", L.NewFunction(luaHashFile))
	mod.RawSetString("hash", hashMod)
}

func luaHashMD5(L *lua.LState) int {
	data := L.CheckString(1)
	hash := md5.Sum([]byte(data))
	L.Push(lua.LString(hex.EncodeToString(hash[:])))
	return 1
}

func luaHashSHA256(L *lua.LState) int {
	data := L.CheckString(1)
	hash := sha256.Sum256([]byte(data))
	L.Push(lua.LString(hex.EncodeToString(hash[:])))
	return 1
}

func luaHashFile(L *lua.LState) int {
	path := L.CheckString(1)
	algo := L.OptString(2, "sha256")

	f, err := os.Open(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer f.Close()

	var hash string
	switch algo {
	case "md5":
		h := md5.New()
		io.Copy(h, f)
		hash = hex.EncodeToString(h.Sum(nil))
	case "sha256":
		h := sha256.New()
		io.Copy(h, f)
		hash = hex.EncodeToString(h.Sum(nil))
	default:
		L.Push(lua.LNil)
		L.Push(lua.LString("unsupported algorithm: " + algo))
		return 2
	}

	L.Push(lua.LString(hash))
	return 1
}

// ══════════════════════════════════════════════════════════════════════════════
// Log Module (revoco.log.*)
// ══════════════════════════════════════════════════════════════════════════════

func registerLogModule(L *lua.LState, mod *lua.LTable) {
	logMod := L.NewTable()
	logMod.RawSetString("debug", L.NewFunction(luaLogDebug))
	logMod.RawSetString("info", L.NewFunction(luaLogInfo))
	logMod.RawSetString("warn", L.NewFunction(luaLogWarn))
	logMod.RawSetString("error", L.NewFunction(luaLogError))
	mod.RawSetString("log", logMod)
}

func luaLogDebug(L *lua.LState) int {
	msg := L.CheckString(1)
	fmt.Printf("[DEBUG] %s\n", msg)
	return 0
}

func luaLogInfo(L *lua.LState) int {
	msg := L.CheckString(1)
	fmt.Printf("[INFO] %s\n", msg)
	return 0
}

func luaLogWarn(L *lua.LState) int {
	msg := L.CheckString(1)
	fmt.Printf("[WARN] %s\n", msg)
	return 0
}

func luaLogError(L *lua.LState) int {
	msg := L.CheckString(1)
	fmt.Printf("[ERROR] %s\n", msg)
	return 0
}

// ══════════════════════════════════════════════════════════════════════════════
// Exec Module (revoco.exec)
// ══════════════════════════════════════════════════════════════════════════════

// AllowedBinaries is the list of binaries that plugins can execute.
// This provides a security boundary.
var AllowedBinaries = map[string]bool{
	"exiftool": true,
	"ffmpeg":   true,
	"ffprobe":  true,
	"convert":  true, // ImageMagick
	"identify": true, // ImageMagick
	"magick":   true, // ImageMagick 7
	"gpg":      true,
	"openssl":  true,
}

func registerExecModule(L *lua.LState, mod *lua.LTable) {
	mod.RawSetString("exec", L.NewFunction(luaExec))
}

func luaExec(L *lua.LState) int {
	binary := L.CheckString(1)

	// Security check
	if !AllowedBinaries[binary] {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("binary not allowed: %s", binary)))
		return 2
	}

	// Get arguments
	var args []string
	argsVal := L.Get(2)
	if tbl, ok := argsVal.(*lua.LTable); ok {
		tbl.ForEach(func(_, value lua.LValue) {
			if str, ok := value.(lua.LString); ok {
				args = append(args, string(str))
			}
		})
	}

	// Execute
	cmd := exec.Command(binary, args...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Build result table
	result := L.NewTable()
	result.RawSetString("stdout", lua.LString(stdout.String()))
	result.RawSetString("stderr", lua.LString(stderr.String()))

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.RawSetString("exit_code", lua.LNumber(exitErr.ExitCode()))
		} else {
			result.RawSetString("exit_code", lua.LNumber(-1))
			result.RawSetString("error", lua.LString(err.Error()))
		}
	} else {
		result.RawSetString("exit_code", lua.LNumber(0))
	}

	L.Push(result)
	return 1
}

// ══════════════════════════════════════════════════════════════════════════════
// HTTP Module (revoco.http.*)
// ══════════════════════════════════════════════════════════════════════════════

// AllowedDomains is the list of domains that plugins can make HTTP requests to.
// This provides a security boundary for network access.
var AllowedDomains = map[string]bool{
	// Google services
	"www.googleapis.com":           true,
	"photoslibrary.googleapis.com": true,
	"oauth2.googleapis.com":        true,
	"accounts.google.com":          true,

	// Common cloud APIs
	"api.dropbox.com":        true,
	"content.dropboxapi.com": true,
	"graph.microsoft.com":    true,
	"api.onedrive.com":       true,

	// Photo services
	"api.flickr.com":  true,
	"api.smugmug.com": true,

	// Music services
	"api.spotify.com":     true,
	"api.music.apple.com": true,

	// Generic
	"localhost": true,
	"127.0.0.1": true,
}

// HTTPConfig holds HTTP client configuration for the Lua runtime.
type HTTPConfig struct {
	Timeout       time.Duration
	MaxBodySize   int64
	AllowInsecure bool
}

// DefaultHTTPConfig returns the default HTTP configuration.
func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		Timeout:       30 * time.Second,
		MaxBodySize:   50 * 1024 * 1024, // 50MB
		AllowInsecure: false,
	}
}

var httpConfig = DefaultHTTPConfig()

func registerHTTPModule(L *lua.LState, mod *lua.LTable) {
	httpMod := L.NewTable()
	httpMod.RawSetString("get", L.NewFunction(luaHTTPGet))
	httpMod.RawSetString("post", L.NewFunction(luaHTTPPost))
	httpMod.RawSetString("request", L.NewFunction(luaHTTPRequest))
	mod.RawSetString("http", httpMod)
}

// isDomainAllowed checks if a URL's domain is in the allowlist.
func isDomainAllowed(rawURL string) (bool, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false, "", err
	}

	host := parsed.Hostname()
	if AllowedDomains[host] {
		return true, host, nil
	}

	return false, host, nil
}

func luaHTTPGet(L *lua.LState) int {
	urlStr := L.CheckString(1)

	// Check domain allowlist
	allowed, host, err := isDomainAllowed(urlStr)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("invalid URL: %s", err.Error())))
		return 2
	}
	if !allowed {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("domain not allowed: %s", host)))
		return 2
	}

	// Optional headers table
	var headers map[string]string
	if L.GetTop() >= 2 {
		headersTbl := L.OptTable(2, nil)
		if headersTbl != nil {
			headers = make(map[string]string)
			headersTbl.ForEach(func(key, value lua.LValue) {
				if k, ok := key.(lua.LString); ok {
					if v, ok := value.(lua.LString); ok {
						headers[string(k)] = string(v)
					}
				}
			})
		}
	}

	// Create request
	ctx, cancel := context.WithTimeout(context.Background(), httpConfig.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Add headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make request
	client := &http.Client{Timeout: httpConfig.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer resp.Body.Close()

	// Read body (with size limit)
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpConfig.MaxBodySize))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Build response table
	result := L.NewTable()
	result.RawSetString("status", lua.LNumber(resp.StatusCode))
	result.RawSetString("body", lua.LString(body))

	// Add response headers
	respHeaders := L.NewTable()
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders.RawSetString(k, lua.LString(v[0]))
		}
	}
	result.RawSetString("headers", respHeaders)

	L.Push(result)
	return 1
}

func luaHTTPPost(L *lua.LState) int {
	urlStr := L.CheckString(1)
	bodyStr := L.OptString(2, "")
	contentType := L.OptString(3, "application/json")

	// Check domain allowlist
	allowed, host, err := isDomainAllowed(urlStr)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("invalid URL: %s", err.Error())))
		return 2
	}
	if !allowed {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("domain not allowed: %s", host)))
		return 2
	}

	// Optional headers table
	var headers map[string]string
	if L.GetTop() >= 4 {
		headersTbl := L.OptTable(4, nil)
		if headersTbl != nil {
			headers = make(map[string]string)
			headersTbl.ForEach(func(key, value lua.LValue) {
				if k, ok := key.(lua.LString); ok {
					if v, ok := value.(lua.LString); ok {
						headers[string(k)] = string(v)
					}
				}
			})
		}
	}

	// Create request
	ctx, cancel := context.WithTimeout(context.Background(), httpConfig.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", urlStr, strings.NewReader(bodyStr))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make request
	client := &http.Client{Timeout: httpConfig.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer resp.Body.Close()

	// Read body
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpConfig.MaxBodySize))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Build response table
	result := L.NewTable()
	result.RawSetString("status", lua.LNumber(resp.StatusCode))
	result.RawSetString("body", lua.LString(body))

	respHeaders := L.NewTable()
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders.RawSetString(k, lua.LString(v[0]))
		}
	}
	result.RawSetString("headers", respHeaders)

	L.Push(result)
	return 1
}

// luaHTTPRequest is a flexible HTTP request function
func luaHTTPRequest(L *lua.LState) int {
	opts := L.CheckTable(1)

	// Extract options
	method := getStringField(opts, "method")
	if method == "" {
		method = "GET"
	}
	urlStr := getStringField(opts, "url")
	if urlStr == "" {
		L.Push(lua.LNil)
		L.Push(lua.LString("url is required"))
		return 2
	}

	// Check domain allowlist
	allowed, host, err := isDomainAllowed(urlStr)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("invalid URL: %s", err.Error())))
		return 2
	}
	if !allowed {
		L.Push(lua.LNil)
		L.Push(lua.LString(fmt.Sprintf("domain not allowed: %s", host)))
		return 2
	}

	bodyStr := getStringField(opts, "body")

	// Create request
	ctx, cancel := context.WithTimeout(context.Background(), httpConfig.Timeout)
	defer cancel()

	var bodyReader io.Reader
	if bodyStr != "" {
		bodyReader = strings.NewReader(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), urlStr, bodyReader)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Add headers
	headersTbl := opts.RawGetString("headers")
	if tbl, ok := headersTbl.(*lua.LTable); ok {
		tbl.ForEach(func(key, value lua.LValue) {
			if k, ok := key.(lua.LString); ok {
				if v, ok := value.(lua.LString); ok {
					req.Header.Set(string(k), string(v))
				}
			}
		})
	}

	// Make request
	client := &http.Client{Timeout: httpConfig.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer resp.Body.Close()

	// Read body
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpConfig.MaxBodySize))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Build response table
	result := L.NewTable()
	result.RawSetString("status", lua.LNumber(resp.StatusCode))
	result.RawSetString("body", lua.LString(body))

	respHeaders := L.NewTable()
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders.RawSetString(k, lua.LString(v[0]))
		}
	}
	result.RawSetString("headers", respHeaders)

	L.Push(result)
	return 1
}
