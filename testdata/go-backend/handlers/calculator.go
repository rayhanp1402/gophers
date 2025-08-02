package handlers

import (
	"encoding/json"
	"net/http"

	"example.com/go-backend/models"
)

type Calculator struct{}

func CalculateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.CalculationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	calc := Calculator{}
	result := calc.CalculateSum(req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (c Calculator) CalculateSum(req models.CalculationRequest) models.CalculationResult {
	return models.CalculationResult{Sum: req.A + req.B}
}