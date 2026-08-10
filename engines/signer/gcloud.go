package signer_engines

import (
	"context"
	"encoding/pem"
	"fmt"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/ecadlabs/gotez/v2/crypt"
	"github.com/tez-capital/tezpay/engines/signer/x509"
	"github.com/trilitech/tzgo/codec"
	"github.com/trilitech/tzgo/signer"
	"github.com/trilitech/tzgo/tezos"
)

type GCSigner struct {
	ctx         context.Context
	cryptPubKey crypt.PublicKey
	key         tezos.Key
	source      string
}

func InitGCSigner(ctx context.Context, kmsKeySource string) (s *GCSigner, err error) {
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return s, err
	}
	defer client.Close()

	pk, err := client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{Name: kmsKeySource})
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode([]byte(pk.Pem))
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	publicKey, err := crypt.NewPublicKeyFrom(pub)
	if err != nil {
		return nil, err
	}

	key, err := tezos.ParseKey(publicKey.String())
	if err != nil {
		return nil, err
	}

	return &GCSigner{
		ctx:         ctx,
		source:      kmsKeySource,
		cryptPubKey: publicKey,
		key:         key,
	}, nil
}

func (s *GCSigner) GetId() string {
	return "GCSigner"
}

func (s *GCSigner) GetPKH() tezos.Address {
	return s.key.Address()
}

func (s *GCSigner) GetKey() tezos.Key {
	return s.key
}

func (s *GCSigner) signDigest(digest []byte) (tezos.Signature, error) {
	client, err := kms.NewKeyManagementClient(s.ctx)
	if err != nil {
		return tezos.InvalidSignature, err
	}
	defer client.Close()

	req := kmspb.AsymmetricSignRequest{
		Name: s.source,
		Data: digest,
	}
	resp, err := client.AsymmetricSign(s.ctx, &req)
	if err != nil {
		return tezos.InvalidSignature, fmt.Errorf("AsymmetricSign: %w", err)
	}

	sig, err := crypt.NewSignatureFromBytes(resp.Signature, s.cryptPubKey)
	if err != nil {
		return tezos.InvalidSignature, fmt.Errorf("NewSignatureFromBytes: %w", err)
	}

	tzSig, err := tezos.ParseSignature(sig.String())
	if err != nil {
		return tezos.InvalidSignature, fmt.Errorf("ParseSignature: %w", err)
	}

	return tzSig, nil
}

func (s *GCSigner) Sign(op *codec.Op) error {
	sig, err := s.signDigest(op.Digest())
	if err != nil {
		return err
	}
	op.Signature = sig
	return nil
}

func (s *GCSigner) GetSigner() signer.Signer {
	return &gcSignerAdapter{s: s}
}

// gcSignerAdapter adapts GCSigner to the tzgo signer.Signer interface required
// by commands (e.g. reveal, transfer) that route through the generic transactor
// options rather than calling SignerEngine.Sign directly.
type gcSignerAdapter struct {
	s *GCSigner
}

func (a *gcSignerAdapter) ListAddresses(_ context.Context) ([]tezos.Address, error) {
	return []tezos.Address{a.s.GetPKH()}, nil
}

func (a *gcSignerAdapter) GetKey(_ context.Context, addr tezos.Address) (tezos.Key, error) {
	if !a.s.GetPKH().Equal(addr) {
		return tezos.InvalidKey, signer.ErrAddressMismatch
	}
	return a.s.GetKey(), nil
}

func (a *gcSignerAdapter) SignMessage(_ context.Context, addr tezos.Address, msg string) (tezos.Signature, error) {
	if !a.s.GetPKH().Equal(addr) {
		return tezos.InvalidSignature, signer.ErrAddressMismatch
	}
	op := codec.NewOp().WithBranch(tezos.ZeroBlockHash).WithContents(&codec.FailingNoop{
		Arbitrary: msg,
	})
	digest := tezos.Digest(op.Bytes())
	return a.s.signDigest(digest[:])
}

func (a *gcSignerAdapter) SignOperation(_ context.Context, addr tezos.Address, op *codec.Op) (tezos.Signature, error) {
	if !a.s.GetPKH().Equal(addr) {
		return tezos.InvalidSignature, signer.ErrAddressMismatch
	}
	if err := a.s.Sign(op); err != nil {
		return tezos.InvalidSignature, err
	}
	return op.Signature, nil
}

func (a *gcSignerAdapter) SignBlock(_ context.Context, addr tezos.Address, head *codec.BlockHeader) (tezos.Signature, error) {
	if !a.s.GetPKH().Equal(addr) {
		return tezos.InvalidSignature, signer.ErrAddressMismatch
	}
	if head.Signature.IsValid() {
		return head.Signature, nil
	}
	sig, err := a.s.signDigest(head.Digest())
	if err != nil {
		return tezos.InvalidSignature, err
	}
	sig.Type = tezos.SignatureTypeGeneric
	head.Signature = sig
	return sig, nil
}
