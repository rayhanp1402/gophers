package models

type CalculationRequest struct {
	A int `json:"a"`
	B int `json:"b"`
}

type CalculationResult struct {
	Sum int `json:"sum"`
}
