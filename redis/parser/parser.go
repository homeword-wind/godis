package parser

import (
	"bufio"
	"errors"
	"io"
	"runtime/debug"
	"strconv"
	"strings"

	"godis/interface/redis"
	"godis/lib/logger"
	"godis/redis/protocol/asserts"
)

type Payload struct {
	Data redis.Reply
	Err  error
}

type readState struct {
	readingMultiLine  bool
	expectedArgsCount int
	msgType           byte
	args              [][]byte
	bulkLen           int64
	readingRepl       bool
}

/*
RESP
- Simple String: "+OK\r\n"
- Error: "-ERR Invalid Syntax\r\n
- Integer: ":1\r\n"
- Bulk String: "$"
- Array / Multi Bulk String: "*"

Bulk String have 2 line >>
*/
func readLine(bufReader *bufio.Reader, state *readState) ([]byte, bool, error) {
	var (
		msg []byte
		err error
	)
	if state.bulkLen == 0 {
		msg, err = bufReader.ReadBytes('\n')
		if err != nil {
			return nil, true, err
		}
		if len(msg) == 0 || msg[len(msg)-2] != '\r' {
			return nil, false, errors.New("protocol error: " + string(msg))
		}
	} else {
		bulkLen := state.bulkLen + 2
		if state.readingRepl {
			bulkLen -= 2
		}
		msg = make([]byte, bulkLen)
		_, err = io.ReadFull(bufReader, msg)
		if err != nil {
			return nil, true, err
		}
		state.bulkLen = 0
	}

	return msg, false, nil
}

func parseBulkHeader(msg []byte, state *readState) error {
	var err error
	state.bulkLen, err = strconv.ParseInt(string(msg[1:len(msg)-2]), 10, 64)
	if err != nil {
		return errors.New("protocol error: " + string(msg))
	}

	if state.bulkLen == -1 {
		return nil
	} else if state.bulkLen > 0 {
		state.msgType = msg[0]
		state.readingMultiLine = true
		state.expectedArgsCount = 1
		state.args = make([][]byte, 0, 1)
		return nil
	} else {
		return errors.New("protocol error: " + string(msg))
	}
}

func parseMultiBulkHeader(msg []byte, state *readState) error {
	expectedLine, err := strconv.ParseUint(string(msg[1:len(msg)-2]), 10, 32)
	if err != nil {
		return errors.New("protocol error: " + string(msg))
	}

	if expectedLine == 0 {
		state.expectedArgsCount = 0
		return nil
	} else if expectedLine > 0 {
		// first line of multi bulk protocol
		state.msgType = msg[0]
		state.readingMultiLine = true
		state.expectedArgsCount = int(expectedLine)
		state.args = make([][]byte, 0, expectedLine)
		return nil
	} else {
		return errors.New("protocol error: " + string(msg))
	}
}

func parseSingleLineReply(msg []byte) (redis.Reply, error) {
	str := strings.TrimSuffix(string(msg), "\r\n")
	var result redis.Reply
	switch msg[0] {
	case '+':
		result = asserts.InitStatusReply(str[1:])
	case '-':
		result = asserts.InitErrReply(str[1:])
	case ':':
		val, err := strconv.ParseInt(str[1:], 10, 64)
		if err != nil {
			return nil, errors.New("protocol error: " + string(msg))
		}

		result = asserts.InitIntReply(val)
	default:
		strs := strings.Split(str, " ")
		args := make([][]byte, len(strs))
		for i, s := range strs {
			args[i] = []byte(s)
		}
		result = asserts.InitMultiBulkReply(args)
	}

	return result, nil
}

func parse(reader io.Reader, ch chan<- *Payload) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(err, string(debug.Stack()))
		}
	}()

	bufReader := bufio.NewReader(reader)

	var (
		state readState
		err   error
		msg   []byte
	)
	for {
		var ioErr bool
		// read line
		msg, ioErr, err = readLine(bufReader, &state)
		if err != nil {
			// encounter io err, stop read
			if ioErr {
				ch <- &Payload{
					Err: err,
				}
				close(ch)
				return
			}
			// protocol err, reset read state
			ch <- &Payload{
				Err: err,
			}
			state = readState{}
			continue
		}
		// parse line
		if !state.readingMultiLine {
			// multi bulk protocol
			if msg[0] == '*' {
				err = parseMultiBulkHeader(msg, &state)
				if err != nil {
					ch <- &Payload{
						Err: errors.New("protocol error: " + string(msg)),
					}
					state = readState{}
					continue
				}
			} else if msg[0] == '$' {
				err = parseBulkHeader(msg, &state)
				if err != nil {
					ch <- &Payload{
						Err: errors.New("protocol error: " + string(msg)),
					}
					state = readState{}
					continue
				}

				if state.bulkLen == -1 {
					ch <- &Payload{
						Data: &asserts.NullBulkReply{},
					}
					state = readState{}
					continue
				}
			} else {
				result, err := parseSingleLineReply(msg)
				ch <- &Payload{
					Data: result,
					Err:  err,
				}
				state = readState{}
				continue
			}
		} else {
			// TODO
			// receive following bulk protocol
		}
	}
}
