package main

import "strings"

func (s *Server) serviceOpenAPI() map[string]any {
	security := []map[string]any{{"ApiKey": []string{}}}
	responses := map[string]any{"200": map[string]string{"description": "JSON envelope: code, msg, data. 邮件结果必须检查 data.status；accepted 表示 SMTP 接受，不代表送达。unknown 禁止盲目重发。"}, "400": map[string]string{"description": "参数错误"}, "401": map[string]string{"description": "凭证无效、到期或撤销"}, "403": map[string]string{"description": "权限不足或项目停用"}, "404": map[string]string{"description": "邮件记录不存在于本项目"}, "409": map[string]string{"description": "幂等键对应的内容不同"}, "429": map[string]string{"description": "请求或邮件配额超限"}, "503": map[string]string{"description": "服务暂不可用"}}
	spec := map[string]any{"openapi": "3.0.3", "info": map[string]string{"title": "DuoYu ID Service API", "version": "1.2.0"}, "servers": []map[string]string{{"url": s.origin}}, "paths": map[string]any{
		"/api/service/app":       map[string]any{"get": map[string]any{"summary": "读取所属项目资料（app:read）", "security": security, "responses": responses}},
		"/api/service/mail":      map[string]any{"post": map[string]any{"summary": "发送一封项目邮件（mail:send）", "description": "项目后端调用。text 使用默认模板；html 非空时直接作为 HTML 正文。业务验证码由项目生成与验证。加密队列持久化，完成后清除正文；仅安全的临时失败最多尝试 3 次。检查 data.status，queued 表示已入队。", "security": security, "parameters": []map[string]any{{"name": "Idempotency-Key", "in": "header", "required": true, "schema": map[string]any{"type": "string", "minLength": 8, "maxLength": 128}}}, "requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"to", "subject"}, "properties": map[string]any{"to": map[string]any{"type": "string", "format": "email"}, "subject": map[string]any{"type": "string", "description": "单行，最多 200 UTF-8 字节"}, "text": map[string]any{"type": "string", "description": "最多 16 KiB；text/html 至少一个非空"}, "html": map[string]any{"type": "string", "description": "最多 64 KiB；项目生成的完整 HTML"}, "category": map[string]any{"type": "string", "enum": []string{"notification", "verification", "alert"}, "default": "notification"}}}}}}, "responses": responses}},
		"/api/service/mail/{id}": map[string]any{"get": map[string]any{"summary": "查询本项目邮件状态（mail:read）", "security": security, "parameters": []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}}}, "responses": responses}},
	}, "components": map[string]any{"securitySchemes": map[string]any{"ApiKey": map[string]string{"type": "http", "scheme": "bearer"}}}}
	components := spec["components"].(map[string]any)
	components["schemas"] = map[string]any{"APIResponse": map[string]any{"type": "object", "required": []string{"code", "msg", "data", "error", "request_id", "api_version"}, "properties": map[string]any{
		"code": map[string]any{"type": "integer"}, "msg": map[string]any{"type": "string"}, "data": map[string]any{"nullable": true}, "error": map[string]any{"type": "object", "nullable": true, "properties": map[string]any{"code": map[string]string{"type": "string"}, "retryable": map[string]string{"type": "boolean"}}}, "request_id": map[string]string{"type": "string"}, "api_version": map[string]any{"type": "string", "enum": []string{"1"}},
	}}}
	s.extendManagementOpenAPI(spec)
	for path, value := range spec["paths"].(map[string]any) {
		if strings.HasPrefix(path, "/api/service/") {
			for _, operation := range value.(map[string]any) {
				op, ok := operation.(map[string]any)
				if !ok {
					continue
				}
				params, _ := op["parameters"].([]map[string]any)
				op["parameters"] = append(params, map[string]any{"name": "X-Duoyu-Project", "in": "header", "required": true, "description": "目标项目 ID，凭证必须具有该项目访问权", "schema": map[string]string{"type": "string"}})
			}
		}
	}
	for _, path := range spec["paths"].(map[string]any) {
		for _, operation := range path.(map[string]any) {
			op, yes := operation.(map[string]any)
			if !yes {
				continue
			}
			entries, yes := op["responses"].(map[string]any)
			if !yes {
				continue
			}
			for code, response := range entries {
				description := "业务响应"
				switch value := response.(type) {
				case map[string]string:
					description = value["description"]
				case map[string]any:
					if v, ok := value["description"].(string); ok {
						description = v
					}
				}
				entries[code] = map[string]any{"description": description, "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"$ref": "#/components/schemas/APIResponse"}}}}
			}
		}
	}
	return spec
}
