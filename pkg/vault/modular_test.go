package vault

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestModularSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Modular Vault Testing Suite")
}
