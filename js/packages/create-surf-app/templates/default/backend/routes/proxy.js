/**
 * Catch-all proxy to data APIs.
 *
 * Sandbox mode: forwards /proxy/* to OutboundProxy at 127.0.0.1:9999
 * Deployed mode: transparent pass-through /proxy/{path} -> hermod /gateway/v1/{path}
 *
 * IMPORTANT: All proxy middlewares use selfHandleResponse + responseInterceptor
 * to buffer the full upstream response before sending. This forces Express to
 * respond with Content-Length instead of Transfer-Encoding: chunked, which is
 * required for compatibility with the KEDA HTTP interceptor (it drops chunked
 * response bodies when the upstream sends Connection: close).
 */

const { createProxyMiddleware, responseInterceptor } = require('http-proxy-middleware');

const GATEWAY_URL = process.env.GATEWAY_URL;
const APP_TOKEN = process.env.APP_TOKEN;
const IS_DEPLOYED = Boolean(GATEWAY_URL && APP_TOKEN);

// Shared response interceptor: buffer full response so Express sends Content-Length
// instead of Transfer-Encoding: chunked (fixes KEDA HTTP interceptor body-drop bug).
const bufferResponse = responseInterceptor(async (responseBuffer, _proxyRes, _req, _res) => {
  return responseBuffer;
});

function setupProxyRoutes(app) {
  if (IS_DEPLOYED) {
    // Transparent pass-through: /proxy/{path} -> hermod /gateway/v1/{path}
    // pathRewrite strips /proxy prefix and prepends /gateway/v1
    app.use('/proxy', createProxyMiddleware({
      target: GATEWAY_URL,
      changeOrigin: true,
      selfHandleResponse: true,
      pathRewrite: (p) => '/gateway/v1' + p,
      headers: { Authorization: `Bearer ${APP_TOKEN}`, 'Accept-Encoding': 'identity' },
      on: { proxyRes: bufferResponse },
    }));
  } else {
    // Sandbox: forward to OutboundProxy (handles all routing internally)
    app.use(
      createProxyMiddleware({
        target: process.env.DATA_PROXY_BASE ? process.env.DATA_PROXY_BASE.replace(/\/proxy$/, '') : 'http://127.0.0.1:9999',
        changeOrigin: true,
        selfHandleResponse: true,
        pathFilter: '/proxy',
        on: {
          proxyReq: (proxyReq, req) => {
            console.log(`[proxy] >> ${req.method} ${req.originalUrl}`);
          },
          proxyRes: responseInterceptor(async (responseBuffer, proxyRes, req, _res) => {
            const status = proxyRes.statusCode;
            const tag = status >= 400 ? 'ERR' : 'OK';
            console.log(`[proxy] << ${status} ${tag} ${req.method} ${req.originalUrl} bytes=${responseBuffer.length}`);
            return responseBuffer;
          }),
          error: (err, req, res) => {
            console.error(`[proxy] !! ${req.method} ${req.originalUrl} error: ${err.message}`);
            if (!res.headersSent) res.status(502).json({ error: err.message });
          },
        },
      })
    );
  }
}

module.exports = { setupProxyRoutes };
