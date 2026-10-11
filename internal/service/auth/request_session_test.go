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

package auth

import (
	"context"
	"github.com/apache/answer/internal/entity"
	"testing"
)

type requestSessionRepo struct {
	AuthRepo
	stored           *entity.UserCacheInfo
	requestWrites    int
	persistentWrites int
}

func (r *requestSessionRepo) SetRequestUserCacheInfo(_ context.Context, _ string, info *entity.UserCacheInfo) error {
	r.stored = info
	r.requestWrites++
	return nil
}
func (r *requestSessionRepo) SetUserCacheInfo(_ context.Context, _, _ string, _ *entity.UserCacheInfo) error {
	r.persistentWrites++
	return nil
}
func (r *requestSessionRepo) GetUserCacheInfo(_ context.Context, _ string) (*entity.UserCacheInfo, error) {
	return r.stored, nil
}
func (r *requestSessionRepo) GetUserStatus(_ context.Context, _ string) (*entity.UserCacheInfo, error) {
	return &entity.UserCacheInfo{UserStatus: entity.UserStatusSuspended, EmailStatus: entity.EmailStatusAvailable, RoleID: 1}, nil
}
func TestRequestSessionDoesNotBecomePersistentOnStatusRefresh(t *testing.T) {
	repo := &requestSessionRepo{}
	service := NewAuthService(repo, nil)
	token, err := service.SetRequestUserCacheInfo(context.Background(), &entity.UserCacheInfo{UserID: "owner"})
	if err != nil || token == "" {
		t.Fatalf("create request session: %v", err)
	}
	info, err := service.GetUserCacheInfo(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if !info.RequestScoped || info.UserStatus != entity.UserStatusSuspended {
		t.Fatal("request scope or account status lost")
	}
	if repo.persistentWrites != 0 || repo.requestWrites != 2 {
		t.Fatal("request session entered persistent session storage")
	}
}
