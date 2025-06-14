package amf

import (
	"fmt"
	"io"
	"math"
)

// Abstract external boilerplate
func (d *Decoder) decodeAbstractMessage(r io.Reader) (result Object, err error) {
	result = make(Object)

	if err = d.decodeExternal(r, &result,
		[]string{"body", "clientId", "destination", "headers", "messageId", "timeStamp", "timeToLive"},
		[]string{"clientIdBytes", "messageIdBytes"}); err != nil {
		return result, fmt.Errorf("unable to decode abstract external: %w", err)
	}

	return
}

// DSA
func (d *Decoder) decodeAsyncMessageExt(r io.Reader) (result Object, err error) {
	return d.decodeAsyncMessage(r)
}

func (d *Decoder) decodeAsyncMessage(r io.Reader) (result Object, err error) {
	result, err = d.decodeAbstractMessage(r)
	if err != nil {
		return result, fmt.Errorf("unable to decode abstract for async: %w", err)
	}

	if err = d.decodeExternal(r, &result, []string{"correlationId", "correlationIdBytes"}); err != nil {
		return result, fmt.Errorf("unable to decode async external: %w", err)
	}

	return
}

// DSK
func (d *Decoder) decodeAcknowledgeMessageExt(r io.Reader) (result Object, err error) {
	return d.decodeAcknowledgeMessage(r)
}

func (d *Decoder) decodeAcknowledgeMessage(r io.Reader) (result Object, err error) {
	result, err = d.decodeAsyncMessage(r)
	if err != nil {
		return result, fmt.Errorf("unable to decode async for ack: %w", err)
	}

	if err = d.decodeExternal(r, &result); err != nil {
		return result, fmt.Errorf("unable to decode ack external: %w", err)
	}

	return
}

// flex.messaging.io.ArrayCollection
func (d *Decoder) decodeArrayCollection(r io.Reader) (any, error) {
	result, err := d.DecodeAmf3(r)
	if err != nil {
		return result, fmt.Errorf("cannot decode child of array collection: %w", err)
	}

	return result, nil
}

func (d *Decoder) decodeExternal(r io.Reader, obj *Object, fieldSets ...[]string) (err error) {
	var flagSet []uint8
	var reservedPosition uint8
	var fieldNames []string

	flagSet, err = readFlags(r)
	if err != nil {
		return fmt.Errorf("unable to read flags: %w", err)
	}

	for i, flags := range flagSet {
		if i < len(fieldSets) {
			fieldNames = fieldSets[i]
		} else {
			fieldNames = []string{}
		}

		reservedPosition = uint8(len(fieldNames))

		for p, field := range fieldNames {
			flagBit := uint8(math.Exp2(float64(p)))
			if (flags & flagBit) != 0 {
				tmp, err := d.DecodeAmf3(r)
				if err != nil {
					return fmt.Errorf(
						"unable to decode external field %s %d %d (%#v): %w",
						field,
						i,
						p,
						flagSet,
						err,
					)
				}
				(*obj)[field] = tmp
			}
		}

		if (flags >> reservedPosition) != 0 {
			for j := reservedPosition; j < 6; j++ {
				if ((flags >> j) & 0x01) != 0 {
					field := fmt.Sprintf("extra_%d_%d", i, j)
					tmp, err := d.DecodeAmf3(r)
					if err != nil {
						return fmt.Errorf(
							"unable to decode post-external field %d %d (%#v): %w",
							i,
							j,
							flagSet,
							err,
						)
					}
					(*obj)[field] = tmp
				}
			}
		}
	}

	return err
}

func readFlags(r io.Reader) (result []uint8, err error) {
	for {
		flag, err := ReadByte(r)
		if err != nil {
			return result, fmt.Errorf("unable to read flags: %w", err)
		}

		result = append(result, flag)
		if (flag & 0x80) == 0 {
			break
		}
	}

	return
}
