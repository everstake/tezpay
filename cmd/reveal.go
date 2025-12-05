package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/tez-capital/tezpay/constants"
	"github.com/trilitech/tzgo/codec"
	"github.com/trilitech/tzgo/rpc"
	"github.com/trilitech/tzgo/tezos"
)

var revealCmd = &cobra.Command{
	Use:   "reveal",
	Short: "reveal address",
	Long:  "runs reveal address",
	Run: func(cmd *cobra.Command, args []string) {
		rpcURL := os.Getenv("RPC_URL")
		if rpcURL == "" && len(constants.DEFAULT_RPC_POOL) > 0 {
			rpcURL = constants.DEFAULT_RPC_POOL[0]
		}
		kmsSource := os.Getenv("KMS_KEY_SOURCE")
		err := reveal(cmd.Context(), rpcURL, kmsSource)
		if err != nil {
			log.Printf("reveal failed: %s", err)
		}
	},
}

func reveal(ctx context.Context, rpcURL, kmsSource string) error {
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return errors.Wrap(err, "creating kms client")
	}
	defer client.Close()

	pk, err := client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{Name: kmsSource})
	if err != nil {
		return errors.Wrap(err, "getting public key")
	}

	pemBlock, _ := pem.Decode([]byte(pk.Pem))
	pub, err := x509.ParsePKIXPublicKey(pemBlock.Bytes)
	if err != nil {
		return errors.Wrap(err, "parsing public key")
	}
	defer client.Close()

	key := tezos.Key{
		Type: tezos.KeyTypeEd25519,
		Data: pub.(ed25519.PublicKey),
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	rpcClient, err := rpc.NewClient(rpcURL, httpClient)
	if err != nil {
		return errors.Wrap(err, "creating rpc client")
	}
	chainID, err := rpcClient.GetChainId(ctx)
	if err != nil {
		return errors.Wrap(err, "getting chain id")
	}
	rpcClient.ChainId = chainID
	address := key.Address()
	log.Printf("revealing address: %s", address)

	opt := rpc.DefaultOptions
	opt.Confirmations = 0
	opt.MaxFee = 5000
	opt.Sender = address

	op := codec.NewOp().WithSource(address).WithContents(&CustomReveal{
		Manager: codec.Manager{
			Fee:      300,
			GasLimit: 180,
			Source:   address,
		},
		PublicKey: key,
	}).WithMinFee()

	err = rpcClient.Complete(ctx, op, key)
	if err != nil {
		return errors.Wrap(err, "failed to complete sign")
	}

	req := kmspb.AsymmetricSignRequest{
		Name: kmsSource,
		Data: op.Digest(),
	}
	resp, err := client.AsymmetricSign(ctx, &req)
	if err != nil {
		log.Fatalf("failed to sign: %s", err)
	}

	op.Signature = tezos.Signature{
		Type: tezos.SignatureTypeEd25519,
		Data: resp.Signature,
	}

	opHash, err := rpcClient.Broadcast(context.Background(), op)
	if err != nil {
		return errors.Wrap(err, "broadcast operation")
	}

	log.Printf("broadcast operation hash: %s", opHash)
	return nil
}

func init() {
	RootCmd.AddCommand(revealCmd)
}

type CustomReveal struct {
	codec.Manager
	PublicKey tezos.Key `json:"public_key"`
}

func (o CustomReveal) Kind() tezos.OpType {
	return tezos.OpTypeReveal
}

func (o CustomReveal) MarshalJSON() ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte('{')
	buf.WriteString(`"kind":`)
	buf.WriteString(strconv.Quote(o.Kind().String()))
	buf.WriteByte(',')
	o.Manager.EncodeJSON(buf)
	buf.WriteString(`,"public_key":`)
	buf.WriteString(strconv.Quote(o.PublicKey.String()))
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (o CustomReveal) EncodeBuffer(buf *bytes.Buffer, p *tezos.Params) error {
	buf.WriteByte(o.Kind().TagVersion(p.OperationTagsVersion))
	o.Manager.EncodeBuffer(buf, p)
	buf.Write(o.PublicKey.Bytes())
	buf.Write([]byte{0})
	return nil
}

func (o *CustomReveal) DecodeBuffer(buf *bytes.Buffer, p *tezos.Params) (err error) {
	if err = ensureTagAndSize(buf, o.Kind(), p.OperationTagsVersion); err != nil {
		return
	}
	if err = o.Manager.DecodeBuffer(buf, p); err != nil {
		return
	}
	if err = o.PublicKey.DecodeBuffer(buf); err != nil {
		return
	}
	return
}

func (o CustomReveal) MarshalBinary() ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	err := o.EncodeBuffer(buf, tezos.DefaultParams)
	return buf.Bytes(), err
}

func (o *CustomReveal) UnmarshalBinary(data []byte) error {
	return o.DecodeBuffer(bytes.NewBuffer(data), tezos.DefaultParams)
}

func ensureTagAndSize(buf *bytes.Buffer, typ tezos.OpType, ver int) error {
	if buf == nil {
		return io.ErrShortBuffer
	}

	tag, err := buf.ReadByte()
	if err != nil {
		// unread so the caller is able to repair
		buf.UnreadByte()
		return err
	}

	if tag != typ.TagVersion(ver) {
		// unread so the caller is able to repair
		buf.UnreadByte()
		return fmt.Errorf("invalid tag %d for op type %s", tag, typ)
	}

	// don't fail size checks for undefined ops
	sz := typ.MinSizeVersion(ver)
	if buf.Len() < sz-1 {
		fmt.Printf("short buffer for tag %d for op type %s: exp=%d got=%d\n", tag, typ,
			sz-1, buf.Len())
		buf.UnreadByte()
		return io.ErrShortBuffer
	}

	return nil
}
