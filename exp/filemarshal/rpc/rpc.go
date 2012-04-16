package gobrpc

import (
	"code.google.com/p/rog-go/exp/filemarshal"
	"io"
	"net/rpc"
)

type clientCodec struct {
	c   io.Closer
	enc filemarshal.Encoder
	dec filemarshal.Decoder
}

func NewClientCodec(conn io.ReadWriteCloser, enc filemarshal.Encoder, dec filemarshal.Decoder) rpc.ClientCodec {
	return &clientCodec{conn, filemarshal.NewEncoder(enc), filemarshal.NewDecoder(dec)}
}

func (c *clientCodec) WriteRequest(r *rpc.Request, body interface{}) error {
	if err := c.enc.Encode(r); err != nil {
		return err
	}
	return c.enc.Encode(body)
}

func (c *clientCodec) ReadResponseHeader(r *rpc.Response) error {
	return c.dec.Decode(r)
}

func (c *clientCodec) ReadResponseBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *clientCodec) Close() error {
	return c.c.Close()
}

type serverCodec struct {
	c   io.Closer
	enc filemarshal.Encoder
	dec filemarshal.Decoder
}

func NewServerCodec(conn io.ReadWriteCloser, enc filemarshal.Encoder, dec filemarshal.Decoder) rpc.ServerCodec {
	return &serverCodec{conn, filemarshal.NewEncoder(enc), filemarshal.NewDecoder(dec)}
}

func (c *serverCodec) ReadRequestHeader(r *rpc.Request) error {
	return c.dec.Decode(r)
}

func (c *serverCodec) ReadRequestBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *serverCodec) WriteResponse(r *rpc.Response, body interface{}) error {
	if err := c.enc.Encode(r); err != nil {
		return err
	}
	return c.enc.Encode(body)
}

func (c *serverCodec) Close() error {
	return c.c.Close()
}
