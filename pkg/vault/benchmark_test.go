package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

// Benchmark key validation performance
func BenchmarkDefaultKeyValidator_ValidateKeys(b *testing.B) {
	validator := NewDefaultKeyValidator()
	keys := make([]string, 5)
	for i := 0; i < 5; i++ {
		keys[i] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("test-key-%d", i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateKeys(keys, 3)
	}
}

func BenchmarkDefaultKeyValidator_ValidateBase64Key(b *testing.B) {
	validator := NewDefaultKeyValidator()
	key := base64.StdEncoding.EncodeToString([]byte("test-key-content"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateBase64Key(key)
	}
}

func BenchmarkStrictKeyValidator_ValidateKeys(b *testing.B) {
	validator := NewStrictKeyValidator(32)
	keys := make([]string, 5)
	for i := 0; i < 5; i++ {
		keyData := make([]byte, 32)
		for j := range keyData {
			keyData[j] = byte(i + j)
		}
		keys[i] = base64.StdEncoding.EncodeToString(keyData)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateKeys(keys, 3)
	}
}

// Benchmark key validation with various key counts
func BenchmarkKeyValidation_ScaleUp(b *testing.B) {
	validator := NewDefaultKeyValidator()
	keyCounts := []int{1, 5, 10, 25, 50, 100}

	for _, keyCount := range keyCounts {
		b.Run(fmt.Sprintf("keys-%d", keyCount), func(b *testing.B) {
			keys := make([]string, keyCount)
			for i := 0; i < keyCount; i++ {
				keys[i] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("key-%d", i)))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = validator.ValidateKeys(keys, keyCount/2)
			}
		})
	}
}

// Benchmark unseal strategy performance
func BenchmarkDefaultUnsealStrategy_Unseal(b *testing.B) {
	strategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), nil)
	mockClient := NewMockVaultClient()
	mockClient.SetSealed(false) // Make it fast by avoiding actual unsealing

	keys := []string{
		base64.StdEncoding.EncodeToString([]byte("key1")),
		base64.StdEncoding.EncodeToString([]byte("key2")),
		base64.StdEncoding.EncodeToString([]byte("key3")),
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = strategy.Unseal(ctx, mockClient, keys, 3)
	}
}

func BenchmarkRetryUnsealStrategy_Success(b *testing.B) {
	baseStrategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), nil)
	retryPolicy := NewDefaultRetryPolicy()
	strategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

	mockClient := NewMockVaultClient()
	mockClient.SetSealed(false) // Immediate success

	keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = strategy.Unseal(ctx, mockClient, keys, 1)
	}
}

func BenchmarkRetryUnsealStrategy_WithRetries(b *testing.B) {
	baseStrategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), nil)
	retryPolicy := &DefaultRetryPolicy{
		maxAttempts: 3,
		baseDelay:   1 * time.Microsecond, // Very fast for benchmark
		maxDelay:    10 * time.Microsecond,
	}
	strategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

	keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockClient := NewMockVaultClient()
		mockClient.SetFailSealStatus(true) // Force retries
		_, _ = strategy.Unseal(ctx, mockClient, keys, 1)
	}
}

// Benchmark client operations
func BenchmarkMockVaultClient_Operations(b *testing.B) {
	client := NewMockVaultClient()
	client.SetSealed(false)
	ctx := context.Background()

	b.Run("IsSealed", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = client.IsSealed(ctx)
		}
	})

	b.Run("GetSealStatus", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = client.GetSealStatus(ctx)
		}
	})

	b.Run("HealthCheck", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = client.HealthCheck(ctx)
		}
	})

	b.Run("IsInitialized", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = client.IsInitialized(ctx)
		}
	})
}

// Benchmark concurrent operations
func BenchmarkMockVaultClient_Concurrent(b *testing.B) {
	client := NewMockVaultClient()
	client.SetSealed(false)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = client.IsSealed(ctx)
		}
	})
}

// Benchmark metrics collection overhead
func BenchmarkMockClientMetrics(b *testing.B) {
	metrics := NewMockClientMetrics()

	b.Run("RecordUnsealAttempt", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metrics.RecordUnsealAttempt("test-endpoint", true, time.Microsecond)
		}
	})

	b.Run("RecordHealthCheck", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metrics.RecordHealthCheck("test-endpoint", true, time.Microsecond)
		}
	})

	b.Run("RecordSealStatusCheck", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			metrics.RecordSealStatusCheck("test-endpoint", true, time.Microsecond)
		}
	})
}

// Benchmark error creation and handling
func BenchmarkErrorCreation(b *testing.B) {
	baseErr := fmt.Errorf("base error")

	b.Run("VaultError", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = NewVaultError("operation", "endpoint", baseErr, true)
		}
	})

	b.Run("ValidationError", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = NewValidationError("field", "value", "message")
		}
	})

	b.Run("IsRetryableError", func(b *testing.B) {
		vaultErr := NewVaultError("test", "endpoint", baseErr, true)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = IsRetryableError(vaultErr)
		}
	})
}

// Benchmark base64 operations (common in key handling)
func BenchmarkBase64Operations(b *testing.B) {
	testData := []byte("test-key-data-for-benchmarking")
	encoded := base64.StdEncoding.EncodeToString(testData)

	b.Run("Encode", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = base64.StdEncoding.EncodeToString(testData)
		}
	})

	b.Run("Decode", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = base64.StdEncoding.DecodeString(encoded)
		}
	})

	b.Run("DecodeAndClear", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil {
				// Clear memory (security practice)
				for j := range decoded {
					decoded[j] = 0
				}
			}
		}
	})
}

// Benchmark memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("SliceAppend", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var keys []string
			for j := 0; j < 10; j++ {
				keys = append(keys, fmt.Sprintf("key-%d", j))
			}
		}
	})

	b.Run("SlicePrealloc", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			keys := make([]string, 0, 10)
			for j := 0; j < 10; j++ {
				keys = append(keys, fmt.Sprintf("key-%d", j))
			}
		}
	})

	b.Run("MapCreation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m := make(map[string]int)
			for j := 0; j < 10; j++ {
				m[fmt.Sprintf("key-%d", j)] = j
			}
		}
	})

	b.Run("MapPrealloc", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m := make(map[string]int, 10)
			for j := 0; j < 10; j++ {
				m[fmt.Sprintf("key-%d", j)] = j
			}
		}
	})
}

// Benchmark context operations
func BenchmarkContextOperations(b *testing.B) {
	ctx := context.Background()

	b.Run("WithCancel", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			newCtx, cancel := context.WithCancel(ctx)
			cancel()
			_ = newCtx
		}
	})

	b.Run("WithTimeout", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			newCtx, cancel := context.WithTimeout(ctx, time.Second)
			cancel()
			_ = newCtx
		}
	})

	b.Run("ContextCheck", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			select {
			case <-ctx.Done():
				// Context canceled
			default:
				// Context still valid
			}
		}
	})
}

// Integration benchmark combining multiple operations
func BenchmarkIntegration_FullUnsealFlow(b *testing.B) {
	validator := NewDefaultKeyValidator()
	metrics := NewMockClientMetrics()
	strategy := NewDefaultUnsealStrategy(validator, metrics)

	keys := make([]string, 5)
	for i := 0; i < 5; i++ {
		keyData := make([]byte, 32)
		for j := range keyData {
			keyData[j] = byte(i + j + 1) // Avoid patterns that fail validation
		}
		keys[i] = base64.StdEncoding.EncodeToString(keyData)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewMockVaultClient()
		client.SetSealed(true)

		// This represents a full unseal flow
		_, _ = strategy.Unseal(ctx, client, keys, 3)
	}
}

// Benchmark comparing different validation approaches
func BenchmarkValidationComparison(b *testing.B) {
	keys := make([]string, 10)
	for i := 0; i < 10; i++ {
		keyData := make([]byte, 32)
		for j := range keyData {
			keyData[j] = byte(i + j + 1)
		}
		keys[i] = base64.StdEncoding.EncodeToString(keyData)
	}

	b.Run("DefaultValidator", func(b *testing.B) {
		validator := NewDefaultKeyValidator()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = validator.ValidateKeys(keys, 5)
		}
	})

	b.Run("StrictValidator", func(b *testing.B) {
		validator := NewStrictKeyValidator(32)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = validator.ValidateKeys(keys, 5)
		}
	})
}
