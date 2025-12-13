package models

import "encoding/json"

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type ExampleRequest struct {
	Language string `json:"language" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

type UpdateTranslationRequest struct {
	DescriptionCn string `json:"description_cn"`
}

type NativeParam struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	DescriptionCn string `json:"description_cn"`
}

type UpdateParamsRequest struct {
	Params []NativeParam `json:"params"`
}

type NativeListResponse struct {
	Hash             string          `json:"hash"`
	JHash            *string         `json:"jhash"`
	Name             string          `json:"name"`
	NameSP           string          `json:"name_sp"`
	Namespace        string          `json:"namespace"`
	ApiSet           string          `json:"apiset"`
	ReturnType       string          `json:"return_type"`
	Params           json.RawMessage `json:"params"`
	Build            int             `json:"build_number"`
	SourceAvailable  bool            `json:"source_available"`
	ExampleAvailable bool            `json:"example_available"`
}

type NativeDetailResponse struct {
	NativeListResponse
	DescriptionOriginal string  `json:"description_original"`
	DescriptionCn       *string `json:"description_cn"`
}

type SourceCodeResponse struct {
	Content    string `json:"content"`
	Language   string `json:"lang"`
	SourceType string `json:"type"`
}

type ExampleResponse struct {
	ID       int    `json:"id"`
	Language string `json:"language"`
	Code     string `json:"code"`
}
