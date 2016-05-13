// Copyright (c) 2013-2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/FactomProject/FactomCode/common"
)

// MaxUserAgentLen is the maximum allowed length for the user agent field in a
// version message (MsgVersion).
const MaxUserAgentLen = 2000

// DefaultUserAgent for wire in the stack
const DefaultUserAgent = "/btcwire:0.2.0/"

// NodeState is one of states like candidate, follower, leaderElect, leader and leaderPrev
type NodeState byte

const (
	// NodeCandidate is candidate
	NodeCandidate	NodeState = 0		
	
	// NodeFollower is follwer other than leaderElect and leaderPrev
	NodeFollower	NodeState	= 1		
	
	// NodeLeaderElect is leaderElect
	NodeLeaderElect NodeState = 2	
	
	// NodeLeader is the leader
	NodeLeader	NodeState = 4		
	
	// NodeLeaderPrev is the prev leader
	NodeLeaderPrev NodeState = 8	
)

// MsgVersion implements the Message interface and represents a bitcoin version
// message.  It is used for a peer to advertise itself as soon as an outbound
// connection is made.  The remote peer then uses this information along with
// its own to negotiate.  The remote peer must then respond with a version
// message of its own containing the negotiated values followed by a verack
// message (MsgVerAck).  This exchange must take place before any further
// communication is allowed to proceed.
type MsgVersion struct {
	// Version of the protocol the node is using.
	ProtocolVersion int32

	// Bitfield which identifies the enabled services.
	Services ServiceFlag

	// Time the message was generated.  This is encoded as an int64 on the wire.
	Timestamp time.Time

	// Address of the remote peer.
	AddrYou NetAddress

	// Address of the local peer.
	AddrMe NetAddress

	// Unique value associated with message that is used to detect self
	// connections.
	Nonce uint64

	// The user agent that generated messsage.  This is a encoded as a varString
	// on the wire.  This has a max length of MaxUserAgentLen.
	UserAgent string

	// Last block seen by the generator of the version message.
	LastBlock int32

	// Don't announce transactions to peer.
	DisableRelayTx bool

	// NodeState of this server: NodeCandidate, NodeFollower, NodeLeaderElect, NodeLeader, NodeLeaderPrev
	NodeState NodeState

	// NodeType is one of SERVER, FULL, LIGHT
	NodeType string

	// NodeID is a 32-bit hash uniquely representing a node in factom network
	NodeID string

	// NodeSig is the signature of its ID and can be verified by other peers
	NodeSig common.Signature
	
	// StartTime is the time when server starts
	StartTime int64

	// Passphrase is the encrypted Passphrase
	Passphrase string
}

// HasService returns whether the specified service is supported by the peer
// that generated the message.
func (msg *MsgVersion) HasService(service ServiceFlag) bool {
	if msg.Services&service == service {
		return true
	}
	return false
}

// AddService adds service as a supported service by the peer generating the
// message.
func (msg *MsgVersion) AddService(service ServiceFlag) {
	msg.Services |= service
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// The version message is special in that the protocol version hasn't been
// negotiated yet.  As a result, the pver field is ignored and any fields which
// are added in new versions are optional.  This also mean that r must be a
// *bytes.Buffer so the number of remaining bytes can be ascertained.
//
// This is part of the Message interface implementation.
func (msg *MsgVersion) BtcDecode(r io.Reader, pver uint32) error {
	buf, ok := r.(*bytes.Buffer)
	if !ok {
		return fmt.Errorf("MsgVersion.BtcDecode reader is not a " +
			"*bytes.Buffer")
	}

	var sec int64
	err := readElements(buf, &msg.ProtocolVersion, &msg.Services, &sec)
	if err != nil {
		return err
	}
	msg.Timestamp = time.Unix(sec, 0)

	err = readNetAddress(buf, pver, &msg.AddrYou, false)
	if err != nil {
		return err
	}

	// Protocol versions >= 106 added a from address, nonce, and user agent
	// field and they are only considered present if there are bytes
	// remaining in the message.
	if buf.Len() > 0 {
		err = readNetAddress(buf, pver, &msg.AddrMe, false)
		if err != nil {
			return err
		}
	}
	if buf.Len() > 0 {
		err = readElement(buf, &msg.Nonce)
		if err != nil {
			return err
		}
	}
	if buf.Len() > 0 {
		userAgent, err := readVarString(buf, pver)
		if err != nil {
			return err
		}
		err = validateUserAgent(userAgent)
		if err != nil {
			return err
		}
		msg.UserAgent = userAgent
	}

	// Protocol versions >= 209 added a last known block field.  It is only
	// considered present if there are bytes remaining in the message.
	if buf.Len() > 0 {
		err = readElement(buf, &msg.LastBlock)
		if err != nil {
			return err
		}
	}

	// There was no relay transactions field before BIP0037Version, but
	// the default behavior prior to the addition of the field was to always
	// relay transactions.
	/*	fmt.Println("buf.len=", buf.Len())
		if buf.Len() > 0 {
			// It's safe to ignore the error here since the buffer has at
			// least one byte and that byte will result in a boolean value
			// regardless of its value.  Also, the wire encoding for the
			// field is true when transactions should be relayed, so reverse
			// it for the DisableRelayTx field.
			var relayTx bool
			//readElement(r, &relayTx)
			err = readElement(buf, &relayTx)
			if err != nil {
				return err
			}
			msg.DisableRelayTx = !relayTx
			fmt.Println("** after reading: msg.DisableRelayTx=", msg.DisableRelayTx)
		}*/

	if buf.Len() > 0 {
		err = readElement(buf, &msg.NodeState)
		if err != nil {
			return err
		}
	}

	if buf.Len() > 0 {
		nodeType, err := readVarString(buf, pver)
		if err != nil {
			return err
		}
		msg.NodeType = nodeType
	}

	if buf.Len() > 0 {
		nodeID, err := readVarString(buf, pver)
		if err != nil {
			return err
		}
		msg.NodeID = nodeID
	}

	if buf.Len() > 0 {
		err = readElement(buf, &msg.NodeSig)
		if err != nil {
			return err
		}
	}
	
	if buf.Len() > 0 {
		err = readElement(buf, &msg.StartTime)
		if err != nil {
			return err
		}
	}

	if buf.Len() > 0 {
		pass, err := readVarString(buf, pver)
		if err != nil {
			return err
		}
		msg.Passphrase = pass
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgVersion) BtcEncode(w io.Writer, pver uint32) error {
	err := validateUserAgent(msg.UserAgent)
	if err != nil {
		return err
	}

	err = writeElements(w, msg.ProtocolVersion, msg.Services,
		msg.Timestamp.Unix())
	if err != nil {
		return err
	}

	err = writeNetAddress(w, pver, &msg.AddrYou, false)
	if err != nil {
		return err
	}

	err = writeNetAddress(w, pver, &msg.AddrMe, false)
	if err != nil {
		return err
	}

	err = writeElement(w, msg.Nonce)
	if err != nil {
		return err
	}

	err = writeVarString(w, pver, msg.UserAgent)
	if err != nil {
		return err
	}

	err = writeElement(w, msg.LastBlock)
	if err != nil {
		return err
	}

	// There was no relay transactions field before BIP0037Version.  Also,
	// the wire encoding for the field is true when transactions should be
	// relayed, so reverse it from the DisableRelayTx field.
	//if pver >= BIP0037Version {
	//err = writeElement(w, !msg.DisableRelayTx)
	//if err != nil {
	//return err
	//}
	//}

	err = writeElement(w, msg.NodeState)
	if err != nil {
		return err
	}

	err = writeVarString(w, pver, msg.NodeType)
	if err != nil {
		return err
	}

	err = writeVarString(w, pver, msg.NodeID)
	if err != nil {
		return err
	}

	err = writeElement(w, msg.NodeSig)
	if err != nil {
		return err
	}

	err = writeElement(w, msg.StartTime)
	if err != nil {
		return err
	}

	err = writeVarString(w, pver, msg.Passphrase)
	if err != nil {
		return err
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgVersion) Command() string {
	return CmdVersion
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgVersion) MaxPayloadLength(pver uint32) uint32 {
	// XXX: <= 106 different

	// Protocol version 4 bytes + services 8 bytes + timestamp 8 bytes +
	// remote and local net addresses + nonce 8 bytes + length of user
	// agent (varInt) + max allowed useragent length + last block 4 bytes +
	// relay transactions flag 1 byte.
	return 33 + (maxNetAddressPayload(pver) * 2) + MaxVarIntPayload +
		MaxUserAgentLen + 64
}

// NewMsgVersion returns a new bitcoin version message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgVersion(me *NetAddress, you *NetAddress, nonce uint64,
	lastBlock int32) *MsgVersion {

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	return &MsgVersion{
		ProtocolVersion: int32(ProtocolVersion),
		Services:        0,
		Timestamp:       time.Unix(time.Now().Unix(), 0),
		AddrYou:         *you,
		AddrMe:          *me,
		Nonce:           nonce,
		UserAgent:       DefaultUserAgent,
		LastBlock:       lastBlock,
		DisableRelayTx:  false,
		NodeType:        "SERVER",
		NodeID:          "SERVER_A",
	}
}

// validateUserAgent checks userAgent length against MaxUserAgentLen
func validateUserAgent(userAgent string) error {
	if len(userAgent) > MaxUserAgentLen {
		str := fmt.Sprintf("user agent too long [len %v, max %v]",
			len(userAgent), MaxUserAgentLen)
		return messageError("MsgVersion", str)
	}
	return nil
}

// AddUserAgent adds a user agent to the user agent string for the version
// message.  The version string is not defined to any strict format, although
// it is recommended to use the form "major.minor.revision" e.g. "2.6.41".
func (msg *MsgVersion) AddUserAgent(name string, version string,
	comments ...string) error {

	newUserAgent := fmt.Sprintf("%s:%s", name, version)
	if len(comments) != 0 {
		newUserAgent = fmt.Sprintf("%s(%s)", newUserAgent,
			strings.Join(comments, "; "))
	}
	newUserAgent = fmt.Sprintf("%s%s/", msg.UserAgent, newUserAgent)
	err := validateUserAgent(newUserAgent)
	if err != nil {
		return err
	}
	msg.UserAgent = newUserAgent
	return nil
}

// SetPassphrase takes a plain string and encrypt it
func (msg *MsgVersion) SetPassphrase(pass string) {
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
    if err != nil {
        fmt.Println("err in SetPassphrase: ", err)
        return
    }
    if hashedPassword != nil {
    	fmt.Println("hashed pwd=", string(hashedPassword), ", len=", len(hashedPassword))
    	msg.Passphrase = string(hashedPassword)
    }
}

// ComparePassphrase compares the decrypted msg.Passphrase with the passed Passphrase
func (msg *MsgVersion) ComparePassphrase(Passphrase string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(msg.Passphrase), []byte(Passphrase))
    if err == nil {
    	return true
    }
    fmt.Println("err in ComparePassphrase: ", err)
    return false
}