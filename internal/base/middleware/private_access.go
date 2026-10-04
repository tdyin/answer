/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"

	"github.com/apache/answer/internal/entity"
	"github.com/gin-gonic/gin"
)

// PrivateAccess is opt-in. The socket peer, never X-Forwarded-For, establishes
// the trusted Serve boundary. Host loopback publication and tailnet policy are
// still required: all processes on the Serve host are trusted operators.
func (am *AuthUserMiddleware) PrivateAccess() gin.HandlerFunc {
	owner := os.Getenv("ANSWER_TAILSCALE_OWNER")
	if owner == "" {
		return func(ctx *gin.Context) { ctx.Next() }
	}
	email := os.Getenv("ANSWER_OWNER_EMAIL")
	proxy, err := netip.ParsePrefix(os.Getenv("ANSWER_TRUSTED_PROXY_CIDR"))
	if err != nil || email == "" {
		panic("private access requires owner email and trusted proxy CIDR")
	}
	origin := os.Getenv("ANSWER_PRIVATE_ORIGIN")
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		panic("private access requires an HTTPS origin")
	}
	secret, err := os.ReadFile(os.Getenv("ANSWER_INTERNAL_TOKEN_FILE"))
	internal := strings.TrimSpace(string(secret))
	if err != nil || len(internal) < 32 || strings.HasPrefix(internal, "replace-") {
		panic("private access requires an internal token file")
	}
	return func(ctx *gin.Context) {
		path := ctx.Request.URL.Path
		if path == "/healthz" {
			ctx.Next()
			return
		}
		// Disable registration regardless of the mutable public-signup setting.
		if path == "/answer/api/v1/user/register/email" {
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}
		supplied := ctx.GetHeader("X-Answer-Internal-Token")
		if supplied != "" {
			if subtle.ConstantTimeCompare([]byte(supplied), []byte(internal)) != 1 {
				ctx.AbortWithStatus(http.StatusForbidden)
				return
			}
			// Internal credentials never map to the human principal.
			ctx.Request.Header.Del("Tailscale-User-Login")
			ctx.Next()
			return
		}
		host, _, err := net.SplitHostPort(ctx.Request.RemoteAddr)
		peer, peerErr := netip.ParseAddr(host)
		if err != nil || peerErr != nil || !proxy.Contains(peer.Unmap()) || ctx.GetHeader("Tailscale-User-Login") != owner {
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}
		if ctx.GetHeader("Sec-Fetch-Site") == "cross-site" || (ctx.GetHeader("Origin") != "" && ctx.GetHeader("Origin") != origin) {
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}
		if ctx.Request.Method != http.MethodGet && ctx.Request.Method != http.MethodHead && ctx.GetHeader("Origin") != origin {
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}
		if path == "/users/login" || path == "/users/register" {
			ctx.Redirect(http.StatusSeeOther, "/")
			ctx.Abort()
			return
		}
		if strings.HasPrefix(path, "/answer/api/v1/user/login/") {
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}
		user, exists, err := am.userCommon.GetByEmail(ctx, email)
		if err != nil || !exists || user.Status != entity.UserStatusAvailable || user.MailStatus != entity.EmailStatusAvailable {
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}
		// Mint a request-scoped native session. Client-supplied bearer tokens cannot
		// change the mapped owner, and the returned token cannot outlive this request.
		token, principal, err := am.userCommon.CacheLoginUserInfo(ctx, user.ID, user.Status, user.MailStatus, "", true)
		if err != nil {
			ctx.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		defer am.authService.RemoveUserCacheInfo(ctx, token)
		defer am.authService.RemoveAdminUserCacheInfo(ctx, token)
		ctx.Set(ctxUUIDKey, principal)
		ctx.Request.Header.Set("Authorization", "Bearer "+token)
		ctx.Header("Cache-Control", "no-store")
		ctx.Next()
	}
}
