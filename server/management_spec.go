package main

func (s *Server) extendManagementOpenAPI(spec map[string]any) {
	paths := spec["paths"].(map[string]any)
	schemes := spec["components"].(map[string]any)["securitySchemes"].(map[string]any)
	schemes["ProjectManagementKey"] = map[string]any{"type": "http", "scheme": "bearer", "description": "独立 dy_mgmt_ 凭证，不能使用业务邮件 Key 或 OIDC client_secret。仅服务端或 CLI 使用。"}
	security := []map[string]any{{"ProjectManagementKey": []string{}}}
	responses := map[string]any{}
	for status, description := range map[string]string{"200": "成功，JSON envelope {code,msg,data}", "400": "字段错误", "401": "管理凭证无效/过期/撤销", "403": "权限或项目范围不足，或请求携带 Origin", "409": "创建幂等键对应的参数发生变化", "410": "项目已经创建，但24小时结果重放窗口已过期", "412": "项目版本已变化，重新 GET 后合并", "428": "缺少 If-Match", "429": "每个管理凭证每分钟最多120次", "503": "服务暂不可用"} {
		responses[status] = map[string]string{"description": description}
	}
	stringSchema := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	array := map[string]any{"type": "array", "items": map[string]string{"type": "string"}}
	properties := map[string]any{"name": map[string]any{"type": "string", "maxLength": 60}, "description": stringSchema("最多1000 UTF-8字节"), "logo": stringSchema("必须使用上传接口返回的 /project-logos/...png"), "public": map[string]string{"type": "boolean", "description": "创建后不可修改"}, "platform": map[string]any{"type": "string", "enum": []string{"web", "spa", "native", "extension"}}, "enabled": map[string]any{"type": "boolean", "default": true}, "restricted": map[string]any{"type": "boolean", "default": false}, "refresh_enabled": map[string]any{"type": "boolean", "default": false}, "redirects": array, "logout_redirects": array, "allowed_origins": array, "login_methods": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": allLoginMethods()}}, "backchannel_logout_uri": stringSchema("后台退出通知端点，空字符串表示不启用")}
	jsonBody := func(props map[string]any, required []string) map[string]any {
		schema := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
		if len(required) > 0 {
			schema["required"] = required
		}
		return map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": schema}}}
	}
	pathID := map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}}
	operation := func(summary string, params []map[string]any, body any) map[string]any {
		op := map[string]any{"summary": summary, "security": security, "responses": responses}
		if params != nil {
			op["parameters"] = params
		}
		if body != nil {
			op["requestBody"] = body
		}
		return op
	}
	get := operation("查询获准项目完整配置，不返回密钥（projects:read）", []map[string]any{pathID}, nil)
	get["description"] = "data 为项目对象，含 revision。响应 ETag 为 revision 的带引号字符串，更新时原样放入 If-Match。"
	create := operation("幂等创建项目（projects:create）", []map[string]any{{"name": "Idempotency-Key", "in": "header", "required": true, "schema": map[string]any{"type": "string", "minLength": 8, "maxLength": 128}}}, jsonBody(properties, []string{"name", "logo", "redirects"}))
	create["description"] = "默认启用、默认全部登录方式。data={id,client_id,secret,revision,issuer}；公共客户端 secret 为空。相同管理凭证与幂等键、相同参数24小时内返回相同创建结果。响应敏感，不写日志。"
	patch := operation("合并更新项目（projects:update）", []map[string]any{pathID, {"name": "If-Match", "in": "header", "required": true, "schema": map[string]string{"type": "string"}}}, jsonBody(properties, []string{}))
	patch["description"] = "只修改提供的字段；拒绝 null/未知字段。public 不可改变。成功后再次 GET 取得新 revision。版本竞争返回412。PUT 使用相同合并语义。"
	paths["/api/management/projects"] = map[string]any{"get": operation("查询可管理的项目列表（projects:read）", []map[string]any{{"name": "page", "in": "query", "schema": map[string]any{"type": "integer", "default": 1}}, {"name": "page_size", "in": "query", "schema": map[string]any{"type": "integer", "default": 20, "maximum": 100}}, {"name": "q", "in": "query", "schema": map[string]string{"type": "string"}}}, nil), "post": create}
	paths["/api/management/projects/{id}"] = map[string]any{"get": get, "patch": patch, "put": patch}
	paths["/api/management/project-logos"] = map[string]any{"post": operation("上传本地项目Logo（projects:create 或 projects:update）", nil, map[string]any{"required": true, "content": map[string]any{"multipart/form-data": map[string]any{"schema": map[string]any{"type": "object", "required": []string{"logo"}, "properties": map[string]any{"logo": map[string]string{"type": "string", "format": "binary", "description": "PNG/JPEG/WebP，最大2MB"}}}}}})}
	paths["/api/management/projects/{id}/rotate-secret"] = map[string]any{"post": operation("显式轮换客户端密钥，旧密钥立即失效（projects:rotate）", []map[string]any{pathID}, nil)}
	roleProps := map[string]any{"name": stringSchema("1–40字"), "role_key": stringSchema("小写字母开头，字母/数字/下划线，最多40位"), "description": stringSchema("1–500字"), "is_default": map[string]string{"type": "boolean"}}
	rolePath := []map[string]any{pathID, {"name": "role", "in": "path", "required": true, "schema": map[string]string{"type": "string"}}}
	paths["/api/management/projects/{id}/roles"] = map[string]any{"get": operation("读取项目角色（projects:read）", []map[string]any{pathID}, nil), "post": operation("创建项目角色（projects:update）", []map[string]any{pathID}, jsonBody(roleProps, []string{"name", "role_key", "description"}))}
	paths["/api/management/projects/{id}/roles/{role}"] = map[string]any{"put": operation("更新角色（projects:update）", rolePath, jsonBody(roleProps, []string{"name", "role_key", "description", "is_default"})), "delete": operation("删除角色并迁移用户（projects:update）", rolePath, jsonBody(map[string]any{"replacement_id": stringSchema("同一项目的其他角色ID")}, []string{"replacement_id"}))}
	for path, field := range map[string]string{"grants": "emails", "blacklist": "entries"} {
		paths["/api/management/projects/{id}/"+path] = map[string]any{"get": operation("读取 "+path+"（projects:read）", []map[string]any{pathID}, nil), "put": operation("替换 "+path+"（projects:update）", []map[string]any{pathID}, jsonBody(map[string]any{field: array}, []string{field}))}
	}
	paths["/api/management/guide"] = map[string]any{"get": operation("读取可复制给 AI 的完整接入说明（projects:read）", nil, nil)}
	paths["/api/management/openapi.json"] = map[string]any{"get": operation("读取 OpenAPI JSON（projects:read）", nil, nil)}
}
