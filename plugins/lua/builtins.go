package lua

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
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
	"sync"
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
	registerZipModule(L, mod)
	registerTarModule(L, mod)

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
	mod.RawSetString("symlink", L.NewFunction(luaSymlink))
	mod.RawSetString("stat", L.NewFunction(luaStat))
	mod.RawSetString("walk", L.NewFunction(luaWalk))
	mod.RawSetString("listDir", L.NewFunction(luaListDir))

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

// ══════════════════════════════════════════════════════════════════════════════
// Extended File Functions (symlink, stat, walk, listDir)
// ══════════════════════════════════════════════════════════════════════════════

// luaSymlink creates a symbolic link: symlink(target, linkName) -> bool, err
func luaSymlink(L *lua.LState) int {
	target := L.CheckString(1)
	linkName := L.CheckString(2)

	if err := os.Symlink(target, linkName); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(lua.LTrue)
	return 1
}

// luaStat returns file metadata: stat(path) -> {name, size, mod_time, mode, is_dir}, err
func luaStat(L *lua.LState) int {
	path := L.CheckString(1)

	info, err := os.Stat(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	tbl := L.NewTable()
	tbl.RawSetString("name", lua.LString(info.Name()))
	tbl.RawSetString("size", lua.LNumber(info.Size()))
	tbl.RawSetString("mod_time", lua.LNumber(info.ModTime().Unix()))
	tbl.RawSetString("mode", lua.LString(info.Mode().String()))
	tbl.RawSetString("is_dir", lua.LBool(info.IsDir()))

	L.Push(tbl)
	return 1
}

// luaWalk recursively lists all files in a directory: walk(dir) -> [{path, name, size, mod_time, is_dir}, ...], err
func luaWalk(L *lua.LState) int {
	dir := L.CheckString(1)

	results := L.NewTable()
	idx := 1

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors, keep walking
		}

		entry := L.NewTable()
		entry.RawSetString("path", lua.LString(path))
		entry.RawSetString("name", lua.LString(info.Name()))
		entry.RawSetString("size", lua.LNumber(info.Size()))
		entry.RawSetString("mod_time", lua.LNumber(info.ModTime().Unix()))
		entry.RawSetString("is_dir", lua.LBool(info.IsDir()))

		results.RawSetInt(idx, entry)
		idx++
		return nil
	})

	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	L.Push(results)
	return 1
}

// luaListDir lists entries in a directory (non-recursive): listDir(path) -> [{name, size, mod_time, is_dir}, ...], err
func luaListDir(L *lua.LState) int {
	dir := L.CheckString(1)

	entries, err := os.ReadDir(dir)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	results := L.NewTable()
	for i, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		tbl := L.NewTable()
		tbl.RawSetString("name", lua.LString(entry.Name()))
		tbl.RawSetString("size", lua.LNumber(info.Size()))
		tbl.RawSetString("mod_time", lua.LNumber(info.ModTime().Unix()))
		tbl.RawSetString("is_dir", lua.LBool(entry.IsDir()))

		results.RawSetInt(i+1, tbl)
	}

	L.Push(results)
	return 1
}

// ══════════════════════════════════════════════════════════════════════════════
// ZIP Module (revoco.zip.*)
// ══════════════════════════════════════════════════════════════════════════════

// zipRegistry holds open ZIP readers mapped by handle ID.
// This avoids passing Go pointers into Lua space.
var (
	zipMu      sync.Mutex
	zipHandles = map[int]*zip.ReadCloser{}
	zipNextID  = 1
)

func registerZipModule(L *lua.LState, mod *lua.LTable) {
	zipMod := L.NewTable()
	zipMod.RawSetString("open", L.NewFunction(luaZipOpen))
	zipMod.RawSetString("list", L.NewFunction(luaZipList))
	zipMod.RawSetString("read", L.NewFunction(luaZipRead))
	zipMod.RawSetString("extract", L.NewFunction(luaZipExtract))
	zipMod.RawSetString("close", L.NewFunction(luaZipClose))
	mod.RawSetString("zip", zipMod)
}

// luaZipOpen opens a ZIP file and returns a numeric handle: zip.open(path) -> handle, err
func luaZipOpen(L *lua.LState) int {
	path := L.CheckString(1)

	r, err := zip.OpenReader(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	zipMu.Lock()
	handle := zipNextID
	zipNextID++
	zipHandles[handle] = r
	zipMu.Unlock()

	L.Push(lua.LNumber(handle))
	return 1
}

// luaZipList lists files in an open ZIP: zip.list(handle) -> [{name, size, compressed_size, mod_time, is_dir}, ...], err
func luaZipList(L *lua.LState) int {
	handle := L.CheckInt(1)

	zipMu.Lock()
	r, ok := zipHandles[handle]
	zipMu.Unlock()

	if !ok {
		L.Push(lua.LNil)
		L.Push(lua.LString("invalid zip handle"))
		return 2
	}

	results := L.NewTable()
	for i, f := range r.File {
		entry := L.NewTable()
		entry.RawSetString("name", lua.LString(f.Name))
		entry.RawSetString("size", lua.LNumber(f.UncompressedSize64))
		entry.RawSetString("compressed_size", lua.LNumber(f.CompressedSize64))
		entry.RawSetString("mod_time", lua.LNumber(f.Modified.Unix()))
		entry.RawSetString("is_dir", lua.LBool(f.FileInfo().IsDir()))

		results.RawSetInt(i+1, entry)
	}

	L.Push(results)
	return 1
}

// luaZipRead reads a file from an open ZIP into a string: zip.read(handle, name) -> content, err
// WARNING: loads entire file into memory. For large files, use zip.extract instead.
func luaZipRead(L *lua.LState) int {
	handle := L.CheckInt(1)
	name := L.CheckString(2)

	zipMu.Lock()
	r, ok := zipHandles[handle]
	zipMu.Unlock()

	if !ok {
		L.Push(lua.LNil)
		L.Push(lua.LString("invalid zip handle"))
		return 2
	}

	// Find the file
	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			defer rc.Close()

			content, err := io.ReadAll(rc)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}

			L.Push(lua.LString(content))
			return 1
		}
	}

	L.Push(lua.LNil)
	L.Push(lua.LString(fmt.Sprintf("file not found in zip: %s", name)))
	return 2
}

// luaZipExtract extracts a file from an open ZIP to disk: zip.extract(handle, name, destPath) -> bool, err
// This operates at the Go level so large files never pass through Lua memory.
func luaZipExtract(L *lua.LState) int {
	handle := L.CheckInt(1)
	name := L.CheckString(2)
	destPath := L.CheckString(3)

	zipMu.Lock()
	r, ok := zipHandles[handle]
	zipMu.Unlock()

	if !ok {
		L.Push(lua.LFalse)
		L.Push(lua.LString("invalid zip handle"))
		return 2
	}

	// Find the file
	for _, f := range r.File {
		if f.Name != name {
			continue
		}

		// If it's a directory, just create it
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				L.Push(lua.LFalse)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LTrue)
			return 1
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Open source in zip
		rc, err := f.Open()
		if err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer rc.Close()

		// Create destination
		dst, err := os.Create(destPath)
		if err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer dst.Close()

		// Stream copy (no Lua memory overhead)
		if _, err := io.Copy(dst, rc); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Preserve modification time
		if !f.Modified.IsZero() {
			_ = os.Chtimes(destPath, f.Modified, f.Modified)
		}

		L.Push(lua.LTrue)
		return 1
	}

	L.Push(lua.LFalse)
	L.Push(lua.LString(fmt.Sprintf("file not found in zip: %s", name)))
	return 2
}

// luaZipClose closes an open ZIP handle: zip.close(handle) -> bool
func luaZipClose(L *lua.LState) int {
	handle := L.CheckInt(1)

	zipMu.Lock()
	r, ok := zipHandles[handle]
	if ok {
		delete(zipHandles, handle)
	}
	zipMu.Unlock()

	if !ok {
		L.Push(lua.LFalse)
		return 1
	}

	r.Close()
	L.Push(lua.LTrue)
	return 1
}

// ══════════════════════════════════════════════════════════════════════════════
// TAR Module (revoco.tar.*)
// ══════════════════════════════════════════════════════════════════════════════

func registerTarModule(L *lua.LState, mod *lua.LTable) {
	tarMod := L.NewTable()
	tarMod.RawSetString("list", L.NewFunction(luaTarList))
	tarMod.RawSetString("extractAll", L.NewFunction(luaTarExtractAll))
	mod.RawSetString("tar", tarMod)
}

// luaTarList lists entries in a .tar.gz without extracting: tar.list(path) -> [{name, size, mod_time, is_dir}, ...], err
func luaTarList(L *lua.LState) int {
	path := L.CheckString(1)

	f, err := os.Open(path)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	results := L.NewTable()
	idx := 1

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		entry := L.NewTable()
		entry.RawSetString("name", lua.LString(hdr.Name))
		entry.RawSetString("size", lua.LNumber(hdr.Size))
		entry.RawSetString("mod_time", lua.LNumber(hdr.ModTime.Unix()))
		entry.RawSetString("is_dir", lua.LBool(hdr.Typeflag == tar.TypeDir))

		results.RawSetInt(idx, entry)
		idx++
	}

	L.Push(results)
	return 1
}

// luaTarExtractAll extracts all files from a .tar.gz: tar.extractAll(tgzPath, destDir) -> [{name, path, size, mod_time}, ...], err
func luaTarExtractAll(L *lua.LState) int {
	tgzPath := L.CheckString(1)
	destDir := L.CheckString(2)

	f, err := os.Open(tgzPath)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	results := L.NewTable()
	idx := 1

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Security: prevent path traversal
		target := filepath.Join(destDir, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue // skip entries that would escape destDir
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}

		case tar.TypeReg:
			// Ensure parent directory
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}

			outFile, err := os.Create(target)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			outFile.Close()

			// Preserve modification time
			if !hdr.ModTime.IsZero() {
				_ = os.Chtimes(target, hdr.ModTime, hdr.ModTime)
			}

			entry := L.NewTable()
			entry.RawSetString("name", lua.LString(hdr.Name))
			entry.RawSetString("path", lua.LString(target))
			entry.RawSetString("size", lua.LNumber(hdr.Size))
			entry.RawSetString("mod_time", lua.LNumber(hdr.ModTime.Unix()))
			results.RawSetInt(idx, entry)
			idx++

		case tar.TypeSymlink:
			// Ensure parent directory
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				continue
			}
			_ = os.Remove(target) // remove if exists
			_ = os.Symlink(hdr.Linkname, target)
		}
	}

	L.Push(results)
	return 1
}
