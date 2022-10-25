// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

package gabbygrove

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"math"
	"testing"
	"time"

	refs "github.com/ssbc/go-ssb-refs"
	ssb "github.com/ssbc/go-ssb-refs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ugorji/go/codec"
	"golang.org/x/crypto/ed25519"
)

var startTime = time.Date(1969, 12, 31, 23, 59, 55, 0, time.UTC).Unix()

func fakeNow() time.Time {
	t := time.Unix(startTime, 0)
	startTime++
	return t
}

func generatePrivateKey(t testing.TB, r io.Reader) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(r)
	if err != nil {
		t.Fatal(err)
	}

	return pub, priv
}

func TestEncoder(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)
	dead := bytes.Repeat([]byte("dead"), 8)
	pubKey, privKey := generatePrivateKey(t, bytes.NewReader(dead))

	authorRef, err := refFromPubKey(pubKey)
	r.NoError(err)

	startTime = time.Date(1969, 12, 31, 23, 59, 55, 0, time.UTC).Unix()
	now = fakeNow

	t.Log("kp:", authorRef.URI())

	var msgs = []interface{}{
		append([]byte{0xff}, []byte("s01mBytz")...),
		map[string]interface{}{
			"type": "test",
			"i":    1,
		},
		map[string]interface{}{
			"type":       "contact",
			"contact":    authorRef,
			"spectating": true,
		},
	}

	wantHex := []string{
		"83585385f6d9041a582101aed3dab65ce9e0d6c50d46fceffb552296ed21b6e0b537a6a0184575ce8f5cbd012483d9041a582103a7ac59b52aff894ba89508b35f445ae90628f6d5f358157e4f45f39b5b3be96b090058408a3739fdb99d91e28552e9a2e22650c14a8cdbfe607cdca5767569db2b1e24caa3c31d65964143dc752e568b05c99e0e97c198885bfb8f3549b9c6ccbc99120549ff7330316d4279747a",
		"83587885d9041a582102ccd8fd8392c1b9d1e3026dea42bec93e04b6f8eceb9af2d591489eb8b831c5e1d9041a582101aed3dab65ce9e0d6c50d46fceffb552296ed21b6e0b537a6a0184575ce8f5cbd022383d9041a58210395cca4fa7b24abc6049683e716292b00c49509be147aa024c06286bd9b7dbda8160158403a7f29f7395cc454c3904de2236eef2c0147496b77c556ade1a08bf57d3e70d2a43a4c723aeb5366d4f073ceeb8b2677e03ec62e49d1647c670d95cc77f9db07567b2269223a312c2274797065223a2274657374227d0a",
		"83587985d9041a5821021aaef1f6980c8d9f3f1ebc84dce391212c2f01cd8861943127cd58ec04bc1bb7d9041a582101aed3dab65ce9e0d6c50d46fceffb552296ed21b6e0b537a6a0184575ce8f5cbd032283d9041a5821037018dbc9080ae947c1eea299b7c08bd88d1964f6e35847aae835ff68c1ee55ec1875015840071b5eec6e3b0fcdcedbfd187f43fc621cded3bf81ad37f67374454b12e3f6c72b44926e1b487b4892bff1082d6514e022ce58253956cd4a38212b46a9777d0c58757b22636f6e74616374223a227373623a666565642f676162627967726f76652d76312f72745061746c7a70344e624644556238375f745649706274496262677454656d6f42684664633650584c303d222c2273706563746174696e67223a747275652c2274797065223a22636f6e74616374227d0a",
	}

	var prevRef BinaryRef
	for msgidx, msg := range msgs {

		e := NewEncoder(privKey)
		e.WithNowTimestamps(true)
		seq := uint64(msgidx + 1)
		tr, msgRef, err := e.Encode(seq, prevRef, msg)
		r.NoError(err, "msg[%02d]Encode failed", msgidx)
		r.NotNil(msgRef)

		got, err := tr.MarshalCBOR()
		r.NoError(err, "msg[%02d]Marshal failed", msgidx)

		want, err := hex.DecodeString(wantHex[msgidx])
		r.NoError(err)

		a.Equal(len(want), len(got), "msg[%02d] wrong msg length", msgidx)
		if !a.Equal(want, got, "msg[%02d] compare failed", msgidx) {
			t.Log("got", hex.EncodeToString(got))
		}

		a.True(tr.Verify(nil), "msg[%02d] did not verify", msgidx)

		prevRef, err = fromRef(tr.Key())
		r.NoError(err)

		var tr2 Transfer
		err = tr2.UnmarshalCBOR(got)
		r.NoError(err, "msg[%02d] test decode failed", msgidx)
		t.Logf("msg[%02d] transfer decode of %d bytes", msgidx, len(got))
		r.NotNil(tr2.Event)
		r.NotNil(tr2.Signature)
		r.NotNil(tr2.Content)

		t.Log("event bytes:", len(tr2.Event))
		t.Log(hex.EncodeToString(tr2.Event))

		var evt Event
		err = evt.UnmarshalCBOR(tr2.Event)
		r.NoError(err, "evt[%02d] unmarshal failed", msgidx)

		a.NotNil(evt.Author, "evt[%02d] has author", msgidx)
		a.Equal(seq, evt.Sequence)

		r.NotEqual(0, evt.Timestamp)
		a.EqualValues(-5+msgidx, evt.Timestamp)
		if msgidx == 0 {
			a.Nil(evt.Previous, "evt[%02d] has no previous", msgidx)
			a.Equal(ContentTypeArbitrary, evt.Content.Type)
		} else {
			a.NotNil(evt.Previous, "evt[%02d] has previous", msgidx)
			a.Equal(ContentTypeJSON, evt.Content.Type)
		}

		a.NotEqual(0, evt.Content.Size)
	}
}

func TestEvtDecode(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	var input = "85d9041a5821024226e0304155aeea683a98882ca5683579e1cdd5505597fb76498bf4c4973b98d9041a582101aed3dab65ce9e0d6c50d46fceffb552296ed21b6e0b537a6a0184575ce8f5cbd032283d9041a58210327d0b22f26328f03ffce2a7c66b2ee27e337ca5d28cdc89ead668f1dd7f0218b186901"

	data, err := hex.DecodeString(input)
	r.NoError(err)
	r.NotNil(data)

	var evt Event
	evtDec := codec.NewDecoder(bytes.NewReader(data), GetCBORHandle())
	err = evtDec.Decode(&evt)
	r.NoError(err, "decode failed")
	a.NotNil(evt.Author)
	a.NotNil(evt.Previous)
	a.EqualValues("ssb:message/gabbygrove-v1/QibgMEFVrupoOpiILKVoNXnhzdVQVZf7dkmL9MSXO5g=", evt.Previous.URI())
	a.EqualValues("ssb:content/gabbygrove-v1/J9CyLyYyjwP_zip8ZrLuJ-M3yl0ozcierWaPHdfwIYs=", evt.Content.Hash.URI())
	a.Equal(uint64(3), evt.Sequence)
	a.EqualValues(-3, evt.Timestamp)
}
func TestEncodeLargestMsg(t *testing.T) {
	r := require.New(t)
	dead := bytes.Repeat([]byte("dead"), 8)
	_, privKey := generatePrivateKey(t, bytes.NewReader(dead))

	startTime = time.Date(1969, 12, 31, 23, 59, 55, 0, time.UTC).Unix()
	now = fakeNow

	largeMsg := bytes.Repeat([]byte("X"), math.MaxUint16)

	e := NewEncoder(privKey)
	seq := uint64(9999999)

	testMr, err := refs.NewMessageRefFromBytes(bytes.Repeat([]byte("b4ut"), 8), ssb.RefAlgoMessageGabby)
	r.NoError(err)

	fakeRef, err := fromRef(testMr)
	r.NoError(err)

	tr, _, err := e.Encode(seq, fakeRef, largeMsg)
	r.NoError(err, "Encode worked!!")
	r.NotNil(tr)

	trcbor, err := tr.MarshalCBOR()
	r.NoError(err)

	t.Log("len evt:", len(tr.Event))
	t.Log("len total-content:", len(trcbor)-math.MaxUint16)

	ioutil.WriteFile(t.Name(), trcbor, 0700)
	var gotTr Transfer
	err = gotTr.UnmarshalCBOR(trcbor)
	r.NoError(err)

}

func TestEncodeTooLarge(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)
	dead := bytes.Repeat([]byte("dead"), 8)
	_, privKey := generatePrivateKey(t, bytes.NewReader(dead))

	tooLargeMsg := bytes.Repeat([]byte("A"), math.MaxUint16+10)

	e := NewEncoder(privKey)
	seq := uint64(1)
	tr, msgRef, err := e.Encode(seq, BinaryRef{}, tooLargeMsg)
	r.Error(err, "Encode worked!!")
	if !a.Zero(tr) {
		trcbor, err := tr.MarshalCBOR()
		r.NoError(err)
		ioutil.WriteFile(t.Name(), trcbor, 0700)
	}
	r.Zero(msgRef)
}

func TestDecodeContentTooLarge(t *testing.T) {
	r := require.New(t)
	// disable the error check in TestEncodeTooLarge to get this data
	var input = "83585385f6d9041a582101aed3dab65ce9e0d6c50d46fceffb552296ed21b6e0b537a6a0184575ce8f5cbd0124830009d9041a582103e083c9bbcf9d9a5e096f3282216242afe6ec263b6b03b88b7aa509a1ef59d3b25840763688ad9221a30cf05ef71d1e7daa2824741fa0981c67114f1cb0c6cbd63edee8f022fd52a2104eeee0d690995a44c362f971fe34c0531689ae8ae6d75a0f0a5a00010009"
	input += string(bytes.Repeat([]byte("41"), math.MaxUint16+10))

	data, err := hex.DecodeString(input)
	r.NoError(err)
	r.NotNil(data)

	var tr Transfer
	err = tr.UnmarshalCBOR(data)
	r.Error(err, "unmarshal of too large object worked")

}

func benchmarkEncoder(i int, b *testing.B) {
	r := require.New(b)

	dead := bytes.Repeat([]byte("dead"), 8)

	pubKey, privKey := generatePrivateKey(b, bytes.NewReader(dead))
	authorRef, err := refFromPubKey(pubKey)
	r.NoError(err)

	e := NewEncoder(privKey)

	mr, err := refs.NewMessageRefFromBytes(bytes.Repeat([]byte("prev"), 8), ssb.RefAlgoMessageGabby)
	r.NoError(err)
	fakeRef, err := fromRef(mr)
	r.NoError(err)

	msg := map[string]interface{}{
		"type":       "contact",
		"contact":    authorRef.URI(),
		"spectating": true,
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {

		for k := i; k > 0; k-- {
			tr, msgRef, err := e.Encode(uint64(k+1), fakeRef, msg)
			r.NoError(err, "msg[%02d]Encode failed")
			r.NotNil(tr)
			r.NotNil(msgRef)
			// r.True(tr.Verify())
		}
	}
}

func BenchmarkEncoder5(b *testing.B)   { benchmarkEncoder(5, b) }
func BenchmarkEncoder500(b *testing.B) { benchmarkEncoder(500, b) }
func BenchmarkEncoder20k(b *testing.B) { benchmarkEncoder(20000, b) }

func benchmarkVerify(i int, b *testing.B) {
	r := require.New(b)

	dead := bytes.Repeat([]byte("dead"), 8)

	_, privKey := generatePrivateKey(b, bytes.NewReader(dead))

	e := NewEncoder(privKey)

	mr, err := refs.NewMessageRefFromBytes(bytes.Repeat([]byte("prev"), 8), ssb.RefAlgoMessageGabby)
	r.NoError(err)
	fakeRef, err := fromRef(mr)
	r.NoError(err)

	msg := true
	var trs []*Transfer
	for k := i; k > 0; k-- {
		tr, msgRef, err := e.Encode(uint64(k+1), fakeRef, msg)
		r.NoError(err, "msg[%02d]Encode failed")
		r.NotNil(tr)
		r.NotNil(msgRef)
		r.True(tr.Verify(nil))
		trs = append(trs, tr)
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, tr := range trs {
			r.True(tr.Verify(nil))
		}
	}
}

func BenchmarkVerify5(b *testing.B)   { benchmarkVerify(5, b) }
func BenchmarkVerify500(b *testing.B) { benchmarkVerify(500, b) }
func BenchmarkVerify20k(b *testing.B) { benchmarkVerify(20000, b) }
