package funpay_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/kostromin59/funpay"
)

func TestFunpay_Request(t *testing.T) {
	t.Run("successful request with cookies", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie(funpay.CookieGoldenKey); err != nil || cookie.Value != "test_key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		resp, err := fp.Request(context.Background(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("unauthorized request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()

		fp := funpay.New("invalid_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.Request(context.Background(), ts.URL)
		if !errors.Is(err, funpay.ErrAccountUnauthorized) {
			t.Errorf("expected ErrAccountUnauthorized, got %v", err)
		}
	})

	t.Run("too many requests", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.Request(context.Background(), ts.URL)
		if !errors.Is(err, funpay.ErrTooManyRequests) {
			t.Errorf("expected ErrTooManyRequests, got %v", err)
		}
	})

	t.Run("bad status code", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.Request(context.Background(), ts.URL)
		if !errors.Is(err, funpay.ErrBadStatusCode) {
			t.Errorf("expected ErrBadStatusCode, got %v", err)
		}
	})

	t.Run("with proxy", func(t *testing.T) {
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer proxy.Close()

		proxyURL, _ := url.Parse(proxy.URL)

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL("http://example.com")
		fp.SetProxy(proxyURL)

		resp, err := fp.Request(context.Background(), "http://example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		fp := funpay.New("test_key", "test_agent")
		_, err := fp.Request(context.Background(), "://invalid.url")
		if err == nil {
			t.Error("expected error for invalid URL, got nil")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := fp.Request(ctx, ts.URL)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("non-RU locale adds prefix", func(t *testing.T) {
		setupTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
            <html>
                <body data-app-data='{"userId":123,"csrf-token":"test","locale":"en"}'>
                    <div class="user-link-name">testuser</div>
                    <div class="badge-balance">100 ₽</div>
                </body>
            </html>
        `)
		}))
		defer setupTS.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(setupTS.URL)

		if err := fp.Update(context.Background()); err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		if fp.Account().Locale() != funpay.LocaleEN {
			t.Fatalf("expected locale EN, got %v", fp.Account().Locale())
		}

		testTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/en/") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer testTS.Close()

		fp.SetBaseURL(testTS.URL)

		resp, err := fp.Request(context.Background(), testTS.URL+"/path")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("cookies are updated from response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "new_cookie", Value: "new_value"})
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.Request(context.Background(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cookies := fp.Cookies()
		found := false
		for _, cookie := range cookies {
			if cookie.Name == "new_cookie" && cookie.Value == "new_value" {
				found = true
				break
			}
		}

		if !found {
			t.Error("cookies were not updated from response")
		}
	})

	t.Run("sets custom HTTP method", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		resp, err := fp.Request(
			context.Background(),
			ts.URL,
			funpay.RequestWithMethod(http.MethodPost),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("sets custom headers", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Custom-Header") != "test-value" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		resp, err := fp.Request(
			context.Background(),
			ts.URL,
			funpay.RequestWithHeaders(map[string]string{"X-Custom-Header": "test-value"}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("sets custom cookies", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("test_cookie")
			if err != nil || cookie.Value != "test_value" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		resp, err := fp.Request(
			context.Background(),
			ts.URL,
			funpay.RequestWithCookies([]*http.Cookie{{Name: "test_cookie", Value: "test_value"}}),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("sets request body", func(t *testing.T) {
		const testBody = "test request body"
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if string(body) != testBody {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		resp, err := fp.Request(
			context.Background(),
			ts.URL,
			funpay.RequestWithBody(strings.NewReader(testBody)),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("uses custom proxy", func(t *testing.T) {
		proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer proxyServer.Close()

		proxyURL, _ := url.Parse(proxyServer.URL)

		targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("request should not reach target server directly")
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer targetServer.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(targetServer.URL)

		resp, err := fp.Request(
			context.Background(),
			targetServer.URL,
			funpay.RequestWithProxy(proxyURL),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("sets user agent header", func(t *testing.T) {
		const userAgent = "custom-user-agent"
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.UserAgent() != userAgent {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", userAgent)
		fp.SetBaseURL(ts.URL)

		resp, err := fp.Request(context.Background(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid HTTP method", func(t *testing.T) {
		fp := funpay.New("test_key", "test_agent")

		_, err := fp.Request(
			context.Background(),
			"http://example.com",
			funpay.RequestWithMethod("INVALID METHOD\n"),
		)

		if err == nil {
			t.Fatal("expected error for invalid HTTP method, got nil")
		}
	})
}

func TestFunpay_RequestHTML(t *testing.T) {
	t.Run("successful request with app data", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
				<html>
					<body data-app-data='{"userId":123,"csrf-token":"test-csrf","locale":"ru"}'>
						<div class="user-link-name">testuser</div>
						<div class="badge-balance">100 ₽</div>
					</body>
				</html>
			`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		doc, err := fp.RequestHTML(context.Background(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if fp.Account().ID() != 123 {
			t.Errorf("expected userID 123, got %d", fp.Account().ID())
		}

		if fp.Account().Username() != "testuser" {
			t.Errorf("expected username 'testuser', got %q", fp.Account().Username())
		}

		if fp.Account().Balance() != 100 {
			t.Errorf("expected balance 100, got %d", fp.Account().Balance())
		}

		if doc == nil {
			t.Error("expected document, got nil")
		}
	})

	t.Run("missing app data", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body></body></html>`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.RequestHTML(context.Background(), ts.URL)
		if !errors.Is(err, funpay.ErrAccountUnauthorized) {
			t.Errorf("expected ErrAccountUnauthorized, got %v", err)
		}
	})

	t.Run("invalid app data json", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body data-app-data="invalid"></body></html>`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.RequestHTML(context.Background(), ts.URL)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("zero userID in app data", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body data-app-data='{"userId":0,"csrf-token":"test","locale":"ru"}'></body></html>`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.RequestHTML(context.Background(), ts.URL)
		if !errors.Is(err, funpay.ErrAccountUnauthorized) {
			t.Errorf("expected ErrAccountUnauthorized, got %v", err)
		}
	})

	t.Run("html parse error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			// Close connection immediately
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("cannot hijack connection")
			}
			conn, _, _ := hj.Hijack()
			conn.Close()
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.RequestHTML(context.Background(), ts.URL)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("updates account info", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
				<html>
					<body data-app-data='{"userId":456,"csrf-token":"new-csrf","locale":"en"}'>
						<div class="user-link-name">updated_user</div>
						<div class="badge-balance">200 ₽</div>
					</body>
				</html>
			`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.RequestHTML(context.Background(), ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if fp.Account().ID() != 456 {
			t.Errorf("expected userID 456, got %d", fp.Account().ID())
		}

		if fp.Account().Username() != "updated_user" {
			t.Errorf("expected username 'updated_user', got %q", fp.Account().Username())
		}

		if fp.Account().Balance() != 200 {
			t.Errorf("expected balance 200, got %d", fp.Account().Balance())
		}

		if fp.Account().Locale() != funpay.LocaleEN {
			t.Errorf("expected locale EN, got %v", fp.Account().Locale())
		}
	})
}

func TestFunpay_UpdateLocale(t *testing.T) {
	t.Run("successful locale update", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("setlocale") != "en" {
				t.Errorf("expected setlocale=en, got %s", r.URL.Query().Get("setlocale"))
			}

			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
							<html>
									<body data-app-data='{"userId":123,"csrf-token":"test","locale":"en"}'>
											<div class="user-link-name">testuser</div>
											<div class="badge-balance">100 ₽</div>
									</body>
							</html>
					`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		setupTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
							<html>
									<body data-app-data='{"userId":123,"csrf-token":"test","locale":"ru"}'>
											<div class="user-link-name">testuser</div>
											<div class="badge-balance">100 ₽</div>
									</body>
							</html>
					`)
		}))
		defer setupTS.Close()

		fp.SetBaseURL(setupTS.URL)
		if err := fp.Update(context.Background()); err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		fp.SetBaseURL(ts.URL)

		err := fp.UpdateLocale(context.Background(), funpay.LocaleEN)
		if err != nil {
			t.Fatalf("UpdateLocale failed: %v", err)
		}

		if fp.Account().Locale() != funpay.LocaleEN {
			t.Errorf("expected locale EN, got %v", fp.Account().Locale())
		}
	})

	t.Run("invalid URL handling", func(t *testing.T) {
		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL("http://invalid.url:12345")

		err := fp.UpdateLocale(context.Background(), funpay.LocaleEN)
		if err == nil {
			t.Fatal("expected error for invalid URL, got nil")
		}
	})

	t.Run("unauthorized request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()

		fp := funpay.New("invalid_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		err := fp.UpdateLocale(context.Background(), funpay.LocaleEN)
		if !errors.Is(err, funpay.ErrAccountUnauthorized) {
			t.Fatalf("expected ErrAccountUnauthorized, got %v", err)
		}
	})

	t.Run("empty app data in response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body></body></html>`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		err := fp.UpdateLocale(context.Background(), funpay.LocaleEN)
		if !errors.Is(err, funpay.ErrAccountUnauthorized) {
			t.Fatalf("expected ErrAccountUnauthorized, got %v", err)
		}
	})
}

func TestFunpay_LotsByUser(t *testing.T) {
	t.Run("successful lots retrieval", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/users/123/") {
				t.Errorf("expected path to end with /users/123/, got %s", r.URL.Path)
			}

			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
				<html>
					<body data-app-data='{"userId":123,"csrf-token":"test","locale":"ru"}'>
						<div class="offer">
							<h3><a href="/games/game1/">Game 1</a></h3>
							<a class="tc-item" href="/lots/lot1?id=1"></a>
							<a class="tc-item" href="/lots/lot2?id=2"></a>
						</div>
						<div class="offer">
							<h3><a href="/games/game2/">Game 2</a></h3>
							<a class="tc-item" href="/lots/lot3?id=3"></a>
						</div>
					</body>
				</html>
			`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		lots, err := fp.LotsByUser(context.Background(), 123)
		if err != nil {
			t.Fatalf("LotsByUser failed: %v", err)
		}

		expected := map[string][]string{
			"game1": {"1", "2"},
			"game2": {"3"},
		}

		if !reflect.DeepEqual(lots, expected) {
			t.Errorf("expected %v, got %v", expected, lots)
		}
	})

	t.Run("invalid user URL", func(t *testing.T) {
		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL("http://invalid.url:12345")

		_, err := fp.LotsByUser(context.Background(), 123)
		if err == nil {
			t.Fatal("expected error for invalid URL, got nil")
		}
	})

	t.Run("unauthorized request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()

		fp := funpay.New("invalid_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.LotsByUser(context.Background(), 123)
		if !errors.Is(err, funpay.ErrAccountUnauthorized) {
			t.Fatalf("expected ErrAccountUnauthorized, got %v", err)
		}
	})

	t.Run("no offers found", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html>
				<body data-app-data='{"userId":123,"csrf-token":"test","locale":"ru"}'></body>
			</html>`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		lots, err := fp.LotsByUser(context.Background(), 123)
		if err != nil {
			t.Fatalf("LotsByUser failed: %v", err)
		}

		if len(lots) != 0 {
			t.Errorf("expected empty map, got %v", lots)
		}
	})

	t.Run("malformed href in offer", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
				<html>
					<body data-app-data='{"userId":123,"csrf-token":"test","locale":"ru"}'>
						<div class="offer">
							<h3><a href=":invalid:url">Game 1</a></h3>
						</div>
					</body>
				</html>
			`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.LotsByUser(context.Background(), 123)
		if err == nil {
			t.Fatal("expected error for malformed URL, got nil")
		}
	})
}

func TestFunpay_LotFields(t *testing.T) {
	t.Run("successful fields retrieval with offerID", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("offer") != "123" {
				t.Errorf("expected offer=123, got %s", r.URL.Query().Get("offer"))
			}

			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
				<html>
					<body data-app-data='{"userId":123,"csrf-token":"test","locale":"ru"}'>
						<form>
							<input type="text" name="title" value="Test Title">
							<input type="checkbox" name="active" checked>
							<textarea name="description">Test Description</textarea>
							<select name="category">
								<option value="1">Category 1</option>
								<option value="2" selected>Category 2</option>
							</select>
						</form>
					</body>
				</html>
			`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		fields, err := fp.LotFields(context.Background(), "", "123")
		if err != nil {
			t.Fatalf("LotFields failed: %v", err)
		}

		expected := funpay.LotFields{
			"title":       {Value: "Test Title"},
			"active":      {Value: "on", Variants: []string{"on", ""}},
			"description": {Value: "Test Description"},
			"category":    {Value: "2", Variants: []string{"1", "2"}},
		}

		if !reflect.DeepEqual(fields, expected) {
			t.Errorf("expected %+v, got %+v", expected, fields)
		}
	})

	t.Run("successful fields retrieval with nodeID", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("node") != "456" {
				t.Errorf("expected node=456, got %s", r.URL.Query().Get("node"))
			}

			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
				<html>
					<body data-app-data='{"userId":123,"csrf-token":"test","locale":"ru"}'>
						<form>
							<input type="text" name="title" value="New Lot">
						</form>
					</body>
				</html>
			`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		fields, err := fp.LotFields(context.Background(), "456", "")
		if err != nil {
			t.Fatalf("LotFields failed: %v", err)
		}

		expected := funpay.LotFields{
			"title": {Value: "New Lot"},
		}

		if !reflect.DeepEqual(fields, expected) {
			t.Errorf("expected %+v, got %+v", expected, fields)
		}
	})

	t.Run("invalid base URL", func(t *testing.T) {
		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL("http://invalid.url:12345")

		_, err := fp.LotFields(context.Background(), "456", "")
		if err == nil {
			t.Fatal("expected error for invalid URL, got nil")
		}
	})

	t.Run("unauthorized request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()

		fp := funpay.New("invalid_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		_, err := fp.LotFields(context.Background(), "456", "")
		if !errors.Is(err, funpay.ErrAccountUnauthorized) {
			t.Fatalf("expected ErrAccountUnauthorized, got %v", err)
		}
	})

	t.Run("empty form fields", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
				<html>
					<body data-app-data='{"userId":123,"csrf-token":"test","locale":"ru"}'>
						<form></form>
					</body>
				</html>
			`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		fields, err := fp.LotFields(context.Background(), "456", "")
		if err != nil {
			t.Fatalf("LotFields failed: %v", err)
		}

		if len(fields) != 0 {
			t.Errorf("expected empty fields, got %+v", fields)
		}
	})

	t.Run("ignores CSRF token field", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
				<html>
					<body data-app-data='{"userId":123,"csrf-token":"test","locale":"ru"}'>
						<form>
							<input type="hidden" name="csrf_token" value="should-be-ignored">
							<input type="text" name="title" value="Test Title">
						</form>
					</body>
				</html>
			`)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		fields, err := fp.LotFields(context.Background(), "456", "")
		if err != nil {
			t.Fatalf("LotFields failed: %v", err)
		}

		if _, exists := fields["csrf-token"]; exists {
			t.Error("expected csrf-token field to be ignored")
		}
	})
}

func TestFunpay_SaveLot(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Проверяем только базовые требования
			if r.Method != http.MethodPost {
				t.Error("expected POST request")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(ts.URL)

		err := fp.SaveLot(context.Background(), funpay.LotFields{
			"offer_id": {Value: "123"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing offer_id", func(t *testing.T) {
		fp := funpay.New("test_key", "test_agent")

		err := fp.SaveLot(context.Background(), funpay.LotFields{})
		if err == nil {
			t.Error("expected error when offer_id is missing")
		}
	})

	t.Run("request error handling", func(t *testing.T) {
		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL("http://unreachable-url")

		err := fp.SaveLot(context.Background(), funpay.LotFields{
			"offer_id": {Value: "123"},
		})
		if err == nil {
			t.Error("expected error when request fails")
		}
	})
}

func TestFunpay_UpdateLots(t *testing.T) {
	t.Run("successful lots update", func(t *testing.T) {
		accountTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
                <html>
                    <body data-app-data='{"userId":456,"csrf-token":"new-csrf","locale":"en"}'>
                        <div class="user-link-name">test_user</div>
                        <div class="badge-balance">100 ₽</div>
                    </body>
                </html>
            `)
		}))
		defer accountTS.Close()

		fp := funpay.New("test_key", "test_agent")
		fp.SetBaseURL(accountTS.URL)

		if err := fp.Update(context.Background()); err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		lotsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/users/456/") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `
                <html>
                     <body data-app-data='{"userId":456,"csrf-token":"new-csrf","locale":"en"}'>
                        <div class="offer">
                            <h3><a href="/games/game1/">Game 1</a></h3>
                            <a class="tc-item" href="/lots/lot1?id=1"></a>
                        </div>
                    </body>
                </html>
            `)
		}))
		defer lotsTS.Close()

		fp.SetBaseURL(lotsTS.URL)
		err := fp.UpdateLots(context.Background())
		if err != nil {
			t.Fatalf("UpdateLots failed: %v", err)
		}

		// Проверяем, что лоты обновились (косвенно через Lots().List())
		if len(fp.Lots().List()) == 0 {
			t.Error("expected lots to be updated")
		}
	})

	t.Run("unauthorized user", func(t *testing.T) {
		fp := funpay.New("test_key", "test_agent")

		err := fp.UpdateLots(context.Background())
		if !errors.Is(err, funpay.ErrAccountUnauthorized) {
			t.Fatalf("expected ErrAccountUnauthorized, got %v", err)
		}
	})
}
