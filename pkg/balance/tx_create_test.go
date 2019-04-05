package balance

import (
	"testing"

	"github.com/safex/gosafex/pkg/account"
)

func TestTxCreate(t *testing.T) {
	var wallet Wallet

	wallet.Address.ViewKey.Public = HexToKey("77837b91924a710adc525deb941670432de30b52fb3f19e0bef8bc7ff67641c5")
	wallet.Address.ViewKey.Private = HexToKey("9fde8d863a3040ff67ccc07c49b55ee4746d4db410fb18bdde7dbd7ccba4180e")
	wallet.Address.SpendKey.Public = HexToKey("09917953e467c5cd62201ea63a93fcd123c754b249cb8e89d4251d67c907b169")
	wallet.Address.SpendKey.Private = HexToKey("e6887bea1e8126e8160ceef01ec35c81dd3e86e9d0e7e3c47087c113731ae508")

	_, _ = wallet.GetBalance()

	addr, _ := account.FromBase58("SFXtzV7tt2KZqvpCWVWauC5Qf16o3dAwLKNd9hCNzoB21ELLNfFjAMjXRhsR3ohT1AeW8j3jL4gfRahR86x6aoiU5hm5ZJj7BSc")
	var extra []byte
	_ = wallet.TxCreateCash([]DestinationEntry{DestinationEntry{10000000000, 0, *addr, false, false}}, 2, 0, 1, extra, true)
	t.Errorf("Failing!")
}
