package cloud

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed assets/rf-dewi-extended-bold.ttf
var heroDisplayFontTTF []byte

const (
	DefaultGoogleClientID = "304576961554-lmtm0m74kaqc62do08kcapqgaht12slr.apps.googleusercontent.com"
	googleAuthEndpoint    = "https://accounts.google.com/o/oauth2/v2/auth"
)

type CloudUser struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	CreatedAt int64  `json:"created_at,omitempty"`
	LastLogin int64  `json:"last_login_at,omitempty"`
}

type CloudSession struct {
	Token     string    `json:"token,omitempty"`
	ExpiresAt int64     `json:"expires_at"`
	User      CloudUser `json:"user"`
}

func googleClientID() string {
	return firstNonEmpty(os.Getenv("MILA_GOOGLE_CLIENT_ID"), DefaultGoogleClientID)
}

func DefaultSessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mila", "cloud_session.json"), nil
}

func SaveSession(sess *CloudSession) error {
	path, err := DefaultSessionPath()
	if err != nil {
		return err
	}
	return SaveSessionTo(path, sess)
}

func SaveSessionTo(path string, sess *CloudSession) error {
	if sess == nil || strings.TrimSpace(sess.Token) == "" {
		return errors.New("session token is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func LoadSavedSession() (*CloudSession, error) {
	path, err := DefaultSessionPath()
	if err != nil {
		return nil, err
	}
	return LoadSessionFrom(path)
}

func LoadSessionFrom(path string) (*CloudSession, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sess CloudSession
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sess.Token) == "" {
		return nil, errors.New("saved session has empty token")
	}
	return &sess, nil
}

func ClearSavedSession() error {
	path, err := DefaultSessionPath()
	if err != nil {
		return err
	}
	return ClearSessionAt(path)
}

func ClearSessionAt(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func generatePKCE() (verifier, challenge string, err error) {
	verifier, err = randomURLSafe(48)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func buildGoogleAuthURL(authEndpoint, clientID, redirectURI, state, codeChallenge string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("prompt", "select_account")
	return authEndpoint + "?" + q.Encode()
}

type callbackOutcome struct {
	code string
	err  error
}

func writeCallbackHTML(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fontB64 := base64.StdEncoding.EncodeToString(heroDisplayFontTTF)
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
  @font-face {
    font-family: 'RF Dewi Extended';
    src: url('data:font/ttf;base64,%s') format('truetype');
    font-weight: 700;
    font-style: normal;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  html, body {
    width: 100%%;
    height: 100%%;
    background: #0c0c0c;
    color: #ffffff;
    overflow: hidden;
    -webkit-font-smoothing: antialiased;
    font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  }
  .backdrop {
    position: fixed;
    inset: 0;
    pointer-events: none;
    overflow: hidden;
    background:
      radial-gradient(62%% 54%% at 24%% 32%%, rgba(166, 227, 161, 0.11) 0%%, rgba(166, 227, 161, 0) 70%%),
      radial-gradient(58%% 52%% at 76%% 68%%, rgba(137, 220, 235, 0.10) 0%%, rgba(137, 220, 235, 0) 72%%),
      radial-gradient(70%% 60%% at 50%% 50%%, rgba(148, 226, 213, 0.05) 0%%, rgba(12, 12, 12, 0) 75%%);
    filter: blur(54px);
    transform: scale(1.12);
  }
  .vignette {
    position: fixed;
    inset: 0;
    pointer-events: none;
    background: radial-gradient(circle at 50%% 50%%, transparent 35%%, rgba(12, 12, 12, 0.72) 100%%);
  }
  main {
    position: relative;
    z-index: 1;
    width: 100%%;
    height: 100%%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 32px;
    text-align: center;
    user-select: none;
  }
  h1 {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(28px, 4.2vw, 52px);
    font-weight: 700;
    line-height: 1.08;
    letter-spacing: -0.02em;
    color: #ffffff;
  }
  p {
    margin-top: 14px;
    max-width: 480px;
    font-size: clamp(14px, 1.3vw, 17px);
    font-weight: 400;
    line-height: 1.45;
    color: #8f8f94;
  }
</style>
</head>
<body>
  <div class="backdrop" aria-hidden="true"></div>
  <div class="vignette" aria-hidden="true"></div>
  <main>
    <h1>%s</h1>
    <p>%s</p>
  </main>
</body>
</html>`, html.EscapeString(title), fontB64, html.EscapeString(title), html.EscapeString(detail))
}

func (c *Client) LoginWithGoogle(ctx context.Context, openBrowser func(string)) (*CloudSession, error) {
	return c.loginWithGoogleConfig(ctx, googleAuthEndpoint, googleClientID(), "", openBrowser)
}

func (c *Client) loginWithGoogleConfig(ctx context.Context, authEndpoint, clientID, sessionPath string, openBrowser func(string)) (*CloudSession, error) {
	if c == nil {
		return nil, errors.New("cloud client is nil")
	}
	if openBrowser == nil {
		return nil, errors.New("browser opener is required")
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errors.New("google client id is empty")
	}

	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("pkce init: %w", err)
	}
	state, err := randomURLSafe(24)
	if err != nil {
		return nil, fmt.Errorf("state init: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("oauth loopback listen: %w", err)
	}
	defer ln.Close()

	redirectURI := fmt.Sprintf("http://%s/callback", ln.Addr().String())
	resultCh := make(chan callbackOutcome, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errParam := strings.TrimSpace(q.Get("error")); errParam != "" {
			writeCallbackHTML(w, http.StatusBadRequest, "Вход отменён", "Можете закрыть эту вкладку и вернуться в приложение Mila.")
			select {
			case resultCh <- callbackOutcome{err: fmt.Errorf("google oauth error: %s", errParam)}:
			default:
			}
			return
		}
		if q.Get("state") != state {
			writeCallbackHTML(w, http.StatusBadRequest, "Ошибка проверки", "Попробуйте авторизоваться заново из приложения Mila.")
			select {
			case resultCh <- callbackOutcome{err: errors.New("oauth state mismatch")}:
			default:
			}
			return
		}
		code := strings.TrimSpace(q.Get("code"))
		if code == "" {
			writeCallbackHTML(w, http.StatusBadRequest, "Код входа не получен", "Попробуйте авторизоваться заново из приложения Mila.")
			select {
			case resultCh <- callbackOutcome{err: errors.New("missing authorization code")}:
			default:
			}
			return
		}
		writeCallbackHTML(w, http.StatusOK, "Вы вошли в Mila", "Можете закрыть вкладку и вернуться обратно в приложение.")
		select {
		case resultCh <- callbackOutcome{code: code}:
		default:
		}
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	authURL := buildGoogleAuthURL(authEndpoint, clientID, redirectURI, state, challenge)
	openBrowser(authURL)

	var code string
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-resultCh:
		if out.err != nil {
			return nil, out.err
		}
		code = out.code
	}

	var sess CloudSession
	err = c.PostJSON(ctx, "/api/v1/auth/google", map[string]string{
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
	}, &sess)
	if err != nil {
		return nil, err
	}
	c.SetToken(sess.Token)
	if sessionPath != "" {
		_ = SaveSessionTo(sessionPath, &sess)
	} else {
		_ = SaveSession(&sess)
	}
	return &sess, nil
}

type meResponse struct {
	User      CloudUser `json:"user"`
	ExpiresAt int64     `json:"expires_at"`
}

func (c *Client) FetchMe(ctx context.Context) (*CloudSession, error) {
	if c == nil {
		return nil, errors.New("cloud client is nil")
	}
	var resp meResponse
	if err := c.GetJSON(ctx, "/api/v1/auth/me", &resp); err != nil {
		return nil, err
	}
	return &CloudSession{
		ExpiresAt: resp.ExpiresAt,
		User:      resp.User,
	}, nil
}

func (c *Client) Logout(ctx context.Context) error {
	if c == nil {
		return errors.New("cloud client is nil")
	}
	err := c.PostJSON(ctx, "/api/v1/auth/logout", nil, nil)
	c.SetToken("")
	_ = ClearSavedSession()
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
		return nil
	}
	return err
}
