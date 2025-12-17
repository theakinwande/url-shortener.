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
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            color: #fff;
        }
        
        .container {
            max-width: 600px;
            width: 90%;
            padding: 40px;
        }
        
        .logo {
            font-size: 3rem;
            font-weight: 800;
            text-align: center;
            margin-bottom: 10px;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .subtitle {
            text-align: center;
            color: #94a3b8;
            margin-bottom: 40px;
            font-size: 1.1rem;
        }
        
        .card {
            background: rgba(255, 255, 255, 0.05);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 40px;
            border: 1px solid rgba(255, 255, 255, 0.1);
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
        }
        
        .form-group {
            margin-bottom: 20px;
        }
        
        label {
            display: block;
            margin-bottom: 8px;
            color: #94a3b8;
            font-size: 0.9rem;
        }
        
        input[type="text"], input[type="url"] {
            width: 100%;
            padding: 16px 20px;
            border: 2px solid rgba(255, 255, 255, 0.1);
            border-radius: 12px;
            background: rgba(255, 255, 255, 0.05);
            color: #fff;
            font-size: 1rem;
            transition: all 0.3s ease;
        }
        
        input:focus {
            outline: none;
            border-color: #3a7bd5;
            box-shadow: 0 0 0 4px rgba(58, 123, 213, 0.2);
        }
        
        input::placeholder {
            color: #64748b;
        }
        
        .btn {
            width: 100%;
            padding: 16px;
            border: none;
            border-radius: 12px;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            color: #fff;
            font-size: 1.1rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s ease;
            margin-top: 10px;
        }
        
        .btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 10px 40px rgba(0, 210, 255, 0.3);
        }
        
        .btn:disabled {
            opacity: 0.7;
            cursor: not-allowed;
            transform: none;
        }
        
        .result {
            margin-top: 30px;
            padding: 20px;
            background: rgba(0, 210, 255, 0.1);
            border-radius: 12px;
            border: 1px solid rgba(0, 210, 255, 0.3);
            display: none;
        }
        
        .result.show {
            display: block;
            animation: fadeIn 0.3s ease;
        }
        
        .result-label {
            color: #94a3b8;
            font-size: 0.85rem;
            margin-bottom: 8px;
        }
        
        .result-url {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .result-url a {
            color: #00d2ff;
            text-decoration: none;
            font-size: 1.1rem;
            word-break: break-all;
        }
        
        .result-url a:hover {
            text-decoration: underline;
        }
        
        .copy-btn {
            padding: 8px 16px;
            background: rgba(255, 255, 255, 0.1);
            border: 1px solid rgba(255, 255, 255, 0.2);
            border-radius: 8px;
            color: #fff;
            cursor: pointer;
            font-size: 0.85rem;
            transition: all 0.2s ease;
            white-space: nowrap;
        }
        
        .copy-btn:hover {
            background: rgba(255, 255, 255, 0.2);
        }
        
        .error {
            margin-top: 20px;
            padding: 15px;
            background: rgba(255, 71, 87, 0.1);
            border: 1px solid rgba(255, 71, 87, 0.3);
            border-radius: 12px;
            color: #ff6b6b;
            display: none;
        }
        
        .error.show {
            display: block;
        }
        
        .stats {
            margin-top: 40px;
            text-align: center;
            color: #64748b;
            font-size: 0.9rem;
        }
        
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(-10px); }
            to { opacity: 1; transform: translateY(0); }
        }
        
        .footer {
            margin-top: 40px;
            text-align: center;
            color: #475569;
            font-size: 0.85rem;
        }
        
        .footer a {
            color: #64748b;
            text-decoration: none;
        }
        
        .footer a:hover {
            color: #94a3b8;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1 class="logo">🔗 Shortener</h1>
        <p class="subtitle">Make your URLs short and sweet</p>
        
        <div class="card">
            <form id="shorten-form">
                <div class="form-group">
                    <label for="url">Enter your long URL</label>
                    <input type="url" id="url" name="url" placeholder="https://example.com/very/long/url" required>
                </div>
                
                <div class="form-group">
                    <label for="alias">Custom alias (optional)</label>
                    <input type="text" id="alias" name="alias" placeholder="my-custom-link" pattern="[a-zA-Z0-9]{3,16}">
                </div>
                
                <button type="submit" class="btn" id="submit-btn">Shorten URL</button>
            </form>
            
            <div class="result" id="result">
                <div class="result-label">Your shortened URL:</div>
                <div class="result-url">
                    <a href="#" id="short-url" target="_blank"></a>
                    <button class="copy-btn" onclick="copyToClipboard()">Copy</button>
                </div>
            </div>
            
            <div class="error" id="error"></div>
        </div>
        
        <div class="footer">
            <p>Built with Go • <a href="/health">API Status</a></p>
        </div>
    </div>
    
    <script>
        const form = document.getElementById('shorten-form');
        const result = document.getElementById('result');
        const error = document.getElementById('error');
        const shortUrl = document.getElementById('short-url');
        const submitBtn = document.getElementById('submit-btn');
        
        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const url = document.getElementById('url').value;
            const alias = document.getElementById('alias').value;
            
            result.classList.remove('show');
            error.classList.remove('show');
            submitBtn.disabled = true;
            submitBtn.textContent = 'Shortening...';
            
            try {
                const body = { url };
                if (alias) body.custom_alias = alias;
                
                const response = await fetch('/api/shorten', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-API-Key': '123'
                    },
                    body: JSON.stringify(body)
                });
                
                const data = await response.json();
                
                if (!response.ok) {
                    throw new Error(data.error || 'Failed to shorten URL');
                }
                
                shortUrl.href = data.short_url;
                shortUrl.textContent = data.short_url;
                result.classList.add('show');
                
            } catch (err) {
                error.textContent = err.message;
                error.classList.add('show');
            } finally {
                submitBtn.disabled = false;
                submitBtn.textContent = 'Shorten URL';
            }
        });
        
        function copyToClipboard() {
            const url = shortUrl.href;
            navigator.clipboard.writeText(url).then(() => {
                const btn = document.querySelector('.copy-btn');
                btn.textContent = 'Copied!';
                setTimeout(() => btn.textContent = 'Copy', 2000);
            });
        }
    </script>
</body>
</html>`

	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, html)
}
