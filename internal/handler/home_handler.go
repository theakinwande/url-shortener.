package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HomeHandler handles the homepage.
type HomeHandler struct {
	baseURL string
}

// NewHomeHandler creates a new home handler.
func NewHomeHandler(baseURL string) *HomeHandler {
	return &HomeHandler{baseURL: baseURL}
}

// Home renders the homepage with a URL shortening form.
func (h *HomeHandler) Home(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>URL Shortener</title>
    <style>
        body { font-family: system-ui, sans-serif; max-width: 500px; margin: 50px auto; padding: 20px; }
        h1 { margin-bottom: 5px; }
        p { color: #666; margin-bottom: 30px; }
        label { display: block; margin-bottom: 5px; font-weight: 500; }
        input { width: 100%; padding: 10px; margin-bottom: 15px; border: 1px solid #ccc; border-radius: 4px; font-size: 16px; }
        button { padding: 12px 24px; background: #000; color: #fff; border: none; border-radius: 4px; cursor: pointer; font-size: 16px; }
        button:hover { background: #333; }
        .result { margin-top: 20px; padding: 15px; background: #f0f0f0; border-radius: 4px; }
        .result a { color: #0066cc; word-break: break-all; }
        .error { color: #c00; margin-top: 15px; }
        .footer { margin-top: 40px; color: #999; font-size: 14px; }
    </style>
</head>
<body>
    <h1>URL Shortener</h1>
    <p>Shorten your long URLs</p>
    
    <form id="form">
        <label for="url">Long URL</label>
        <input type="url" id="url" placeholder="https://example.com/very/long/url" required>
        
        <label for="alias">Custom alias (optional, 3-16 chars)</label>
        <input type="text" id="alias" placeholder="my-link">
        
        <button type="submit">Shorten</button>
    </form>
    
    <div class="result" id="result" style="display:none;">
        <strong>Short URL:</strong><br>
        <a href="#" id="short-url" target="_blank"></a>
    </div>
    
    <div class="error" id="error"></div>
    
    <div class="footer">
        <a href="/health">API Status</a>
    </div>
    
    <script>
        document.getElementById('form').onsubmit = async (e) => {
            e.preventDefault();
            document.getElementById('result').style.display = 'none';
            document.getElementById('error').textContent = '';
            
            const body = { url: document.getElementById('url').value };
            const alias = document.getElementById('alias').value;
            if (alias) body.custom_alias = alias;
            
            try {
                const res = await fetch('/api/shorten', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'X-API-Key': '123' },
                    body: JSON.stringify(body)
                });
                const data = await res.json();
                if (!res.ok) throw new Error(data.error);
                
                document.getElementById('short-url').href = data.short_url;
                document.getElementById('short-url').textContent = data.short_url;
                document.getElementById('result').style.display = 'block';
            } catch (err) {
                document.getElementById('error').textContent = err.message;
            }
        };
    </script>
</body>
</html>`

	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, html)
}
