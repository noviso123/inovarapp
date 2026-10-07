//go:build !js

package webapp

func prepareBudgetSignatureCanvas() bool { return false }
func currentBudgetSignature() string     { return "" }
func clearBudgetSignatureCanvas()        {}
