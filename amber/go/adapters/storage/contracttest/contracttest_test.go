package contracttest_test

import (
	"testing"

	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
	"github.com/0xsj/ingen/amber/adapters/storage/contracttest"
)

func TestMemoryStoreContract(t *testing.T) {
	contracttest.RunStoreContract(t, func(testing.TB) amberstorage.Store {
		return amberstorage.NewMemoryStore()
	})
}
