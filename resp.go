package main

type Value interface {
	isRespValue()
}

type InArray []BulkString

type BulkString []byte
type SimpleString string
type Integer int64
type Array []Value
type SimpleError string

func (BulkString) isRespValue()   {}
func (SimpleString) isRespValue() {}
func (Integer) isRespValue()      {}
func (Array) isRespValue()        {}
func (InArray) isRespValue()      {}
func (SimpleError) isRespValue()  {}
