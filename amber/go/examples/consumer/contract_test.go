package main

import (
	"testing"

	amberstorage "github.com/0xsj/ingen/amber/adapters/storage"
	"github.com/0xsj/ingen/amber/adapters/storage/contracttest"
)

func TestPublicStorageContractHelper(t *testing.T) {
	contracttest.RunStoreContract(t, func(testing.TB) amberstorage.Store {
		return amberstorage.NewMemoryStore()
	})
}
