package balance

import (
	"github.com/safex/gosafex/internal/crypto/derivation"
	"github.com/safex/gosafex/internal/crypto"
	"github.com/safex/gosafex/pkg/account"
	"github.com/safex/gosafex/pkg/safex"

	"fmt"
	"log"
	"sort"
	"math/rand"
	"time"
	"bytes"
)

// Interface for sorting offsets.
type IOffsetsSort []uint64
func (offs IOffsetsSort) Len() int { return len(offs)}
func (offs IOffsetsSort) Swap(i, j int) { offs[i], offs[j] = offs[j], offs[i]}
func (offs IOffsetsSort) Less(i, j int) bool { return offs[i] < offs[j]}


const EncryptedPaymentIdTail byte = 0x8d

func EncryptPaymentId(paymentId [8]byte, pub [32]byte, priv [32]byte) ([8]byte){
	var derivation1 [32]byte
	var hash []byte

	var data [33]byte
	dpub := derivation.Key(pub)
	dpriv := derivation.Key(priv)
	derivation1 = [32]byte(derivation.DeriveKey(&dpub,&dpriv))

	copy(data[0:32], derivation1[:])
	data[32] = EncryptedPaymentIdTail
	hash = crypto.Keccak256(data[:])
	for i := 0; i < 8; i++ {
		paymentId[i] ^= hash[i]
	}

	return paymentId
}

func GetDestinationViewKeyPub(destinations *[]DestinationEntry, changeAddr *account.Address) *account.PublicKey{
	var addr account.Address
	var count uint = 0
	for _, val := range(*destinations) {
		if val.Amount == 0 && val.TokenAmount == 0 {
			continue
		}

		if changeAddr != nil && val.Address.Equals(changeAddr) {
			continue
		}

		if val.Address.Equals(&addr) {
			continue
		}

		if count > 0 {
			return nil
		}

		addr = val.Address
		count++
	}
	if count == 0 && changeAddr != nil {
		return &(changeAddr.ViewKey)
	}
	return &(addr.ViewKey)
}

func AbsoluteOutputOffsetsToRelative(input []uint64) (ret []uint64) {
	ret = input
	if len(input) == 0 {
		return ret
	}
	sort.Sort(IOffsetsSort(ret))
	for i := len(ret) - 1; i != 0; i-- {
		ret[i] -= ret[i-1]
	}
	
	return ret
}

func Find(arr []int, val int) int {
    for i, n := range arr {
        if val == n {
            return i
        }
    }
    return -1
}

func ApplyPermutation(permutation []int, f func(i,j int)) {
	// sanity check
	for i := 0; i < len(permutation); i++ {
		if Find(permutation, i) == -1 {
			panic("Bad permutation")
		}
	}

	for i := 0; i < len(permutation); i++ {
		current := i
		for i != permutation[current] {
			next := permutation[current]
			f(current, next)
			permutation[current] = current
			current = next
		}
		permutation[current] = current
	}
} 

func getTxInVFromTxInToKey(input TxInToKey) (ret *safex.TxinV){
	ret = new(safex.TxinV)

	if input.TokenKey {
		toKey := new(safex.TxinTokenToKey)
		toKey.TokenAmount = input.Amount
		toKey.KeyOffsets = input.KeyOffsets
		copy(toKey.KImage, input.KeyImage[:])
		ret.TxinTokenToKey = toKey
	} else {
		toKey := new(safex.TxinToKey)
		toKey.Amount = input.Amount
		toKey.KeyOffsets = input.KeyOffsets
		copy(toKey.KImage, input.KeyImage[:])
		ret.TxinToKey = toKey
	}

	return ret
}

func getKeyImage(input *safex.TxinV) ([]byte) {
	if input.TxinToKey != nil {
		return input.TxinToKey.KImage
	} else {
		return input.TxinTokenToKey.KImage
	}
}

// As we dont use subaddresses for now, we will here just count current
// std addresses.
func classifyAddress(destinations *[]DestinationEntry,
					 changeAddr *account.Address) (stdAddr, subAddr int){
	countMap := make(map[string]int)
	for _, dest := range *destinations {
		_, ok := countMap[dest.Address.String()]
		if ok {
			countMap[dest.Address.String()] += 1
		} else {
			countMap[dest.Address.String()] = 1
		}
	}

	return len(countMap), 0
}

func (w *Wallet) constructTxWithKey(
	// Keys are obsolete as this is part of wallet
	sources *[]TxSourceEntry,
	destinations *[]DestinationEntry,
	changeAddr *account.Address,
	extra *[]byte,
	tx *safex.Transaction, 
	unlockTime uint64,
	txKey *[32]byte,
	shuffleOuts bool) (r bool) {
	
	// @todo CurrTransactionCheck

	if *sources == nil {
		panic("Empty sources")
	}

	//var amountKeys [][32]byte
	tx.Reset()

	tx.Version = 1
	copy(tx.Extra[:], *extra)

	//var txKeyPub [32]byte

	// @todo Make sure that this is necessary once code started working,
	// @warning This can be crucial thing regarding 
	ok, extraMap := ParseExtra(extra)

	if ok {
		if _, isThere := extraMap[Nonce]; isThere {
			var paymentId [8]byte
			if val, isThere1 := extraMap[NonceEncryptedPaymentId]; isThere1 {
				viewKeyPub := GetDestinationViewKeyPub(destinations, changeAddr)
				if viewKeyPub == nil {
					log.Println("Destinations have to have exactly one output to support encrypted payment ids")
					return false
				}
				var viewKeyPubBytes [32]byte
				copy(viewKeyPubBytes[:], *viewKeyPub)
				paymentId = EncryptPaymentId(val.([8]byte), viewKeyPubBytes, *txKey)
				extraMap[NonceEncryptedPaymentId] = paymentId
			}

		}
		// @todo set extra after public tx key calculation
	} else {
		log.Println("Failed to parse tx extra!")
		return false
	}

	var summaryInputsMoney uint64 = 0
	var summaryInputsToken uint64 = 0
	var idx int = -1

	for _, src := range *sources {
		idx++
		if src.RealOutput >= uint64(len(src.Outputs)) {
			panic("RealOutputIndex (" + string(src.RealOutput) + ") bigger thatn Outputs length (" + string(len(src.Outputs)) + ")")
			return false
		}

		summaryInputsMoney += src.Amount
		summaryInputsToken += src.TokenAmount

		var inputToKey TxInToKey
		inputToKey.TokenKey = src.TokenTx
		if src.TokenTx {
			inputToKey.Amount = src.TokenAmount
		} else {
			inputToKey.Amount = src.Amount
		}

		inputToKey.KeyImage = src.KeyImage

		for _, outputEntry := range src.Outputs {
			inputToKey.KeyOffsets = append(inputToKey.KeyOffsets, outputEntry.Index)
		}
		
		inputToKey.KeyOffsets = AbsoluteOutputOffsetsToRelative(inputToKey.KeyOffsets)
		tx.Vin = append(tx.Vin, getTxInVFromTxInToKey(inputToKey))
	}

	// shuffle destinations
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(*destinations), func(i, j int) { (*destinations)[i], (*destinations)[j] = (*destinations)[j], (*destinations)[i] })

	insOrder := make([]int, len(*sources))

	for index := range insOrder {
		insOrder[index] = index
	}

	sort.Slice(insOrder, func (i,j int) bool {
		kI := getKeyImage(tx.Vin[i])
		kJ := getKeyImage(tx.Vin[j])

		return bytes.Compare(kI, kJ) < 0
	})

	ApplyPermutation(insOrder, func(i,j int) { 
		tx.Vin[i], tx.Vin[j] = tx.Vin[j], tx.Vin[i]
		(*sources)[i], (*sources)[j] = (*sources)[j], (*sources)[i]
	})

	pubTxKey := derivation.ScalarmultBase(*txKey)
	
	// Write to extra
	extraMap[PubKey] = pubTxKey

	// @todo At the moment serializing extra field is put at this place in code
	//		 because there are no other field other pubkey and paymentID in current
	//		 iteration of wallet and at this point everything mentioned is calculated
	//		 however in futur that can be changed, so PAY ATTENTION!!!
	okExtra, tempExtra := SerializeExtra(extraMap)
	if !okExtra {
		log.Println("Serializing extra field failed!")
		return false
	}

	*extra = tempExtra

	summaryOutsMoney := uint64(0)
	summaryOutsTokens := uint64(0)

	outputIndex := 0

	var derivation1 derivation.Key

	for _, dst := range *destinations {
		if changeAddr != nil && dst.Address.Equals(changeAddr) {
			derivation1 = derivation.DeriveKey(derivation.Key(*pubTxKey), derivation.Key(w.Address.ViewKey.Private))
		} else {
			derivation1 = derivation.DeriveKey(derivation.Key(dst.Address.ViewKey.Public), derivation.Key(*txKey))
		}

		outEphemeral := derivation.DerivationToPublicKey(outputIndex, derivation1, derivation.Key(dst.Address.SpendKey.Public))

		out = new(safex.Txout)
		if dst.TokenTx {
			out.TokenAmount = dst.TokenAmount
			out.Amount = 0
			ttk := new(safex.TxoutTargetV)
			ttk1 := new(safex.TxoutTokenToKey)
			ttk.TxoutTokenToKey = ttk1
			copy(ttk1.Key, outEphemeral[:])
			out.Target = ttk
			tx.Vout = append(tx.Vout, out)
		} else {
			out.TokenAmount = 0
			out.Amount = dst.Amount
			ttk := new(safex.TxoutTargetV)
			ttk1 := new(safex.TxoutToKey)
			ttk.TxoutToKey = ttk1
			copy(ttk1.Key, outEphemeral[:])
			out.Target = ttk
		}

		tx.Vout = append(tx.Vout, out)
		outputIndex++
		summaryOutsMoney += dst.Amount
		summaryOutsTokens += dst.TokenAmount
	}
	// @note Here goes logic for additional keys.
	// 		 Additional keys are used when you are sending to multiple subaddresses.
	//		 As Safex Blockchain doesnt support officially subaddresses this is left blank.


	if summaryOutsMoney > summaryInputsMoney {
		log.Prinln("Tx inputs cash (", summaryInputsMoney, ") less than outputs cash (", summaryOutsMoney, ")")
		return false
	}

	if summaryOutsTokens > summaryInputsToken {
		log.Prinln("Tx inputs token (", summaryInputsToken, ") less than outputs token (", summaryOutsTokens, ")")
		return false
	}

	if w.watchOnlyWallet {
		log.Println("Zero secret key, skipping signatures")
		return true
	}

	if tx.Version == 1 {
		
	}


	fmt.Println("YEAH!")
 	return false
}

func (w *Wallet) constructTxAndGetTxKey(
	// Keys are obsolete as this is part of wallet
	sources *[]TxSourceEntry,
	destinations *[]DestinationEntry,
	changeAddr *account.Address,
	extra *[]byte,
	tx *safex.Transaction, 
	unlockTime uint64,
	txKey *[32]byte) (r bool) {

	
	// src/cryptonote_core/cryptonote_tx_utils.cpp bool construct_tx_and_get_tx_key()
	// There are no subaddresses involved, so no additional keys therefore we dont 
	// need to involve anything regarding suaddress hence 
	r = w.constructTxWithKey(sources, destinations, changeAddr, extra, tx, unlockTime, txKey, true)
	return r
}