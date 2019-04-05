package balance

import (
	"github.com/safex/gosafex/internal/crypto/derivation"
	"github.com/safex/gosafex/pkg/safex"
)

/* NOTES:
- There are possible multiple TxPublicKey in transaction, if transaction has outputs
for more than one address. This is omitted in current implementation, to be added in the future.
HINT: additional tx pub keys in extra and derivations.
-

*/

// Must be implemented at some point.
const TX_EXTRA_PADDING_MAX_COUNT = 255
const TX_EXTRA_NONCE_MAX_COUNT = 255
const TX_EXTRA_TAG_PADDING = 0x00
const TX_EXTRA_TAG_PUBKEY = 0x01
const TX_EXTRA_NONCE = 0x02
const TX_EXTRA_MERGE_MINING_TAG = 0x03
const TX_EXTRA_TAG_ADDITIONAL_PUBKEYS = 0x04
const TX_EXTRA_MYSTERIOUS_MINERGATE_TAG = 0xDE
const TX_EXTRA_BITCOIN_HASH = 0x10
const TX_EXTRA_MIGRATION_PUBKEYS = 0x11
const TX_EXTRA_NONCE_PAYMENT_ID = 0x00
const TX_EXTRA_NONCE_ENCRYPTED_PAYMENT_ID = 0x01

func extractTxPubKey(extra []byte) (pubTxKey [32]byte) {
	// @todo Also if serialization is ok
	if extra[0] == TX_EXTRA_TAG_PUBKEY {
		copy(pubTxKey[:], extra[1:33])
	}
	return pubTxKey
}

func (w *Wallet) matchOutput(txOut *safex.Txout, index uint64, der [32]byte, outputKey *[32]byte) bool {
	derivatedPubKey := derivation.KeyDerivation_To_PublicKey(index, derivation.Key(der), w.Address.SpendKey.Public)
	if txOut.Target.TxoutToKey != nil {
		copy(outputKey[:], txOut.Target.TxoutToKey.Key[0:32])
	} else {
		copy(outputKey[:], txOut.Target.TxoutTokenToKey.Key[0:32])
	}

	// Return also outputkey
	return *outputKey == [32]byte(derivatedPubKey)
}

func (w *Wallet) ProcessTransaction(tx *safex.Transaction, minerTx bool) {
	// @todo Process Unconfirmed.
	// Process outputs
	if len(tx.Vout) != 0 {
		pubTxKey := extractTxPubKey(tx.Extra)

		// @todo uniform key structure.
		txPubKeyDerivation := ([32]byte)(derivation.DeriveKey((*derivation.Key)(&pubTxKey), (*derivation.Key)(&w.Address.ViewKey.Private)))

		for index, output := range tx.Vout {
			var outputKey [32]byte
			if !w.matchOutput(output, uint64(index), txPubKeyDerivation, &outputKey) {
				continue
			}

			ephermal_secret := derivation.KeyDerivation_To_PrivateKey(uint64(index), w.Address.SpendKey.Private, derivation.Key(txPubKeyDerivation))
			ephermal_public := derivation.KeyDerivation_To_PublicKey(uint64(index), derivation.Key(txPubKeyDerivation), w.Address.SpendKey.Public)
			keyimage := derivation.GenerateKeyImage(ephermal_public, ephermal_secret)

			if _, ok := w.outputs[keyimage]; !ok {
				w.outputs[keyimage] = Transfer{output, false, minerTx, tx.BlockHeight}
				w.balance.CashLocked += output.Amount
				w.balance.TokenLocked += output.TokenAmount
			}

		}
	}

	if len(tx.Vin) != 0 {
		for _, input := range tx.Vin {
			var kImage [32]byte
			if input.TxinGen != nil {
				continue
			}
			if input.TxinToKey != nil {
				copy(kImage[:], input.TxinToKey.KImage[0:32])

				if val, ok := w.outputs[derivation.Key(kImage)]; ok {
					w.balance.CashLocked -= val.Output.Amount
					val.Spent = true
				}
			} else {
				if input.TxinTokenToKey != nil {
					copy(kImage[:], input.TxinTokenToKey.KImage[0:32])
					if val, ok := w.outputs[derivation.Key(kImage)]; ok {
						w.balance.TokenLocked -= val.Output.TokenAmount
						val.Spent = true
					}
				}
			}
		}
	}
	// Process inputs

}
