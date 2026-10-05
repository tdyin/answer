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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPrivateBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := strings.Repeat("s", 64)
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANSWER_TAILSCALE_OWNER", "owner@example.com")
	t.Setenv("ANSWER_OWNER_EMAIL", "forum@example.com")
	t.Setenv("ANSWER_TRUSTED_PROXY_CIDR", "127.0.0.1/32")
	t.Setenv("ANSWER_PRIVATE_ORIGIN", "https://forum.example.ts.net")
	t.Setenv("ANSWER_INTERNAL_TOKEN_FILE", file)
	router := gin.New()
	router.Use((&AuthUserMiddleware{}).PrivateAccess("", ""))
	router.NoRoute(func(ctx *gin.Context) {
		if ctx.GetHeader("Tailscale-User-Login") != "" {
			t.Error("internal path retained human identity")
		}
		ctx.Status(http.StatusNoContent)
	})
	tests := []struct {
		name, path, peer, owner, token, origin, method string
		status                                         int
	}{
		{name: "missing identity", peer: "127.0.0.1:10", status: 403},
		{name: "wrong identity", peer: "127.0.0.1:10", owner: "other@example.com", status: 403},
		{name: "forged backend identity", peer: "192.0.2.1:10", owner: "owner@example.com", status: 403},
		{name: "bad internal token", peer: "127.0.0.1:10", owner: "owner@example.com", token: "wrong", status: 403},
		{name: "internal retains own auth", peer: "192.0.2.1:10", owner: "owner@example.com", token: secret, status: 204},
		{name: "connector login denied", path: "/answer/api/v1/connector/login/test", token: secret, status: 403},
		{name: "connector callback denied", path: "/answer/api/v1/connector/redirect/test", token: secret, status: 403},
		{name: "connector binding denied", path: "/answer/api/v1/connector/binding/email", token: secret, status: 403},
		{name: "user center signup denied", path: "/answer/api/v1/user-center/sign-up/callback", token: secret, status: 403},
		{name: "user center login denied", path: "/answer/api/v1/user-center/login/redirect", token: secret, status: 403},
		{name: "internal signup denied", path: "/answer/api/v1/user/register/email", token: secret, status: 403},
		{name: "cross origin denied", peer: "127.0.0.1:10", owner: "owner@example.com", origin: "https://evil.example", status: 403},
		{name: "write without origin denied", peer: "127.0.0.1:10", owner: "owner@example.com", method: "POST", status: 403},
		{name: "login redirected", path: "/users/login", peer: "127.0.0.1:10", owner: "owner@example.com", status: 303},
	}
	for _, prefix := range []string{"", "/prefix"} {
		router = gin.New()
		router.Use((&AuthUserMiddleware{}).PrivateAccess(prefix, prefix))
		router.NoRoute(func(ctx *gin.Context) {
			if ctx.GetHeader("Tailscale-User-Login") != "" {
				t.Error("internal path retained human identity")
			}
			ctx.Status(http.StatusNoContent)
		})
		for _, tc := range tests {
			t.Run(prefix+tc.name, func(t *testing.T) {
				path := tc.path
				if path == "" {
					path = "/"
				}
				path = prefix + path
				method := tc.method
				if method == "" {
					method = "GET"
				}
				req := httptest.NewRequest(method, path, nil)
				req.RemoteAddr = tc.peer
				req.Header.Set("Tailscale-User-Login", tc.owner)
				req.Header.Set("X-Answer-Internal-Token", tc.token)
				req.Header.Set("Origin", tc.origin)
				req.Header.Set("X-Forwarded-For", "127.0.0.1")
				res := httptest.NewRecorder()
				router.ServeHTTP(res, req)
				if res.Code != tc.status {
					t.Fatalf("got %d, want %d", res.Code, tc.status)
				}
			})
		}
	}
}
