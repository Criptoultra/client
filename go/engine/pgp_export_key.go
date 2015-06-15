package engine

//
// engine.PGPKeyImportEngine is a class for optionally generating PGP keys,
// and pushing them into the keybase sigchain via the Delegator.
//

import (
	"fmt"

	"github.com/keybase/client/go/libkb"
	keybase1 "github.com/keybase/client/protocol/go"
)

type queryType int

const (
	unset queryType = iota
	fingerprint
	kid
	either
)

type PGPKeyExportEngine struct {
	libkb.Contextified
	arg   keybase1.PGPQuery
	qtype queryType
	res   []keybase1.KeyInfo
	me    *libkb.User
}

func (e *PGPKeyExportEngine) GetPrereqs() EnginePrereqs {
	return EnginePrereqs{
		Session: true,
	}
}

func (e *PGPKeyExportEngine) Name() string {
	return "PGPKeyExportEngine"
}

func (e *PGPKeyExportEngine) RequiredUIs() []libkb.UIKind {
	return []libkb.UIKind{
		libkb.SecretUIKind,
	}
}

func (e *PGPKeyExportEngine) SubConsumers() []libkb.UIConsumer {
	return nil
}

func (e *PGPKeyExportEngine) Results() []keybase1.KeyInfo {
	return e.res
}

func NewPGPKeyExportEngine(arg keybase1.PgpExportArg, g *libkb.GlobalContext) *PGPKeyExportEngine {
	return &PGPKeyExportEngine{
		arg:          arg.Options,
		qtype:        either,
		Contextified: libkb.NewContextified(g),
	}
}

func NewPGPKeyExportByKIDEngine(arg keybase1.PgpExportByKIDArg, g *libkb.GlobalContext) *PGPKeyExportEngine {
	return &PGPKeyExportEngine{
		arg:          arg.Options,
		qtype:        kid,
		Contextified: libkb.NewContextified(g),
	}
}

func NewPGPKeyExportByFingerprintEngine(arg keybase1.PgpExportByFingerprintArg, g *libkb.GlobalContext) *PGPKeyExportEngine {
	return &PGPKeyExportEngine{
		arg:          arg.Options,
		qtype:        fingerprint,
		Contextified: libkb.NewContextified(g),
	}
}

func (e *PGPKeyExportEngine) pushRes(fp libkb.PgpFingerprint, key string, desc string) {
	e.res = append(e.res, keybase1.KeyInfo{
		Fingerprint: fp.String(),
		Key:         key,
		Desc:        desc,
	})
}

func (e *PGPKeyExportEngine) exportPublic() (err error) {
	keys := e.me.GetActivePgpKeys(false)
	for _, k := range keys {
		fp := k.GetFingerprintP()
		s, err := k.Encode()
		if fp == nil || err != nil {
			continue
		}
		if len(e.arg.Query) > 0 {
			var match bool
			switch e.qtype {
			case either:
				match = libkb.KeyMatchesQuery(k, e.arg.Query, e.arg.ExactMatch)
			case fingerprint:
				match = fp.Match(e.arg.Query, e.arg.ExactMatch)
			case kid:
				match = k.GetKid().Match(e.arg.Query, e.arg.ExactMatch)
			}
			if !match {
				continue
			}
		}
		e.pushRes(*fp, s, k.VerboseDescription())
	}
	return
}

func (e *PGPKeyExportEngine) exportSecret(ctx *Context) error {
	ska := libkb.SecretKeyArg{
		Me:         e.me,
		KeyType:    libkb.PGPKeyType,
		KeyQuery:   e.arg.Query,
		ExactMatch: e.arg.ExactMatch,
	}

	key, skb, err := e.G().Keyrings.GetSecretKeyWithPrompt(ctx.LoginContext, ska, ctx.SecretUI, "key export")
	if err != nil {
		if _, ok := err.(libkb.NoSecretKeyError); ok {
			// if no secret key found, don't return an error, just let
			// the result be empty
			return nil
		}
		return err
	}
	fp := key.GetFingerprintP()
	if fp == nil {
		return libkb.BadKeyError{Msg: "no fingerprint found"}
	}

	if _, ok := key.(*libkb.PgpKeyBundle); !ok {
		return libkb.BadKeyError{Msg: "Expected a PGP key"}
	}

	raw := skb.RawUnlockedKey()
	if raw == nil {
		return libkb.BadKeyError{Msg: "can't get raw representation of key"}
	}

	ret, err := libkb.PgpKeyRawToArmored(raw, true)
	if err != nil {
		return err
	}

	e.pushRes(*fp, ret, "")

	return nil
}

func (e *PGPKeyExportEngine) loadMe() (err error) {
	e.me, err = libkb.LoadMe(libkb.LoadUserArg{PublicKeyOptional: true})
	return
}

func (e *PGPKeyExportEngine) Run(ctx *Context) (err error) {

	e.G().Log.Debug("+ PGPKeyExportEngine::Run")
	defer func() {
		e.G().Log.Debug("- PGPKeyExportEngine::Run -> %s", libkb.ErrToOk(err))
	}()

	if e.qtype == unset {
		return fmt.Errorf("PGPKeyExportEngine: query type not set.")
	}

	if err = e.loadMe(); err != nil {
		return
	}

	if e.arg.Secret {
		err = e.exportSecret(ctx)
	} else {
		err = e.exportPublic()
	}

	return
}
