package server

import (
	"fmt"
	"net/http"

	"meguca/auth"
	"meguca/common"
	"meguca/config"
	"meguca/db"
	"meguca/util"
)

func assertHasWSConnection(w http.ResponseWriter, ip string, board string) bool {
	for _, cl := range common.GetByIPAndBoard(ip, board) {
		if cl.IP() != "" {
			return true
		}
	}
	text403(w, errNotConnected)
	return false
}

// Ensure API user is not banned.
func assertNotBannedAPI(w http.ResponseWriter, r *http.Request, board string) (ip string, ok bool) {
	ip, err := auth.GetIP(r)
	if err != nil {
		text400(w, err)
		return
	}
	if auth.IsBanned(board, ip) {
		text403(w, errBanned)
		return
	}
	ok = true
	return
}

func assertBoard(w http.ResponseWriter, r *http.Request, board string) bool {
	if !config.IsBoard(board) {
		serve404(w, r)
		return false
	}
	return true
}

func assertBoardAPI(w http.ResponseWriter, board string) bool {
	if !config.IsBoard(board) {
		text400(w, errInvalidBoard)
		return false
	}
	return true
}

func checkReadOnly(board string, ss *auth.Session) bool {
	if !config.IsReadOnlyBoard(board) {
		return true
	}
	if ss == nil {
		return false
	}
	return ss.Positions.CurBoard >= auth.Moderator
}

func checkPosition(board string, ss *auth.Session, position auth.ModerationLevel) bool {
	if ss == nil {
		return false
	}
	boards := make([]string, 1)
	boards = append(boards, board)

	staff, err := db.GetStaff(nil, boards)

	if err != nil {
		return false
	}

	inPosition := false
	for _, curStaff := range staff {
		if curStaff.UserID == ss.UserID {
			inPosition = curStaff.Position == position
		}
	}

	return inPosition
}

func checkWhitelistOnly(board string, ss *auth.Session) bool {
	if !config.IsWhitelistOnlyBoard(board) {
		return true
	}
	if ss == nil {
		return false
	}
	return checkPosition(board, ss, 2) || ss.Positions.CurBoard >= auth.Moderator
}

func checkRegisteredOnly(board string, ss *auth.Session) bool {
	if !config.IsRegisteredOnlyBoard(board) {
		return true
	}
	if ss == nil {
		return false
	}
	return true
}

// Ensure smilename not used on this board
func assertSmileNameNotUsed(smileName string, board string) bool {
	valid, _ := db.ValidSmileName(smileName, board)
	return valid
}

// Ensure only mods and above can post at read-only boards.
func assertNotReadOnlyAPI(w http.ResponseWriter, board string, ss *auth.Session) bool {
	if !checkReadOnly(board, ss) {
		text403(w, errReadOnly)
		return false
	}
	return true
}

// Ensure only registered users can post.
func assertNotRegisteredOnlyAPI(w http.ResponseWriter, board string, ss *auth.Session) bool {
	if !checkRegisteredOnly(board, ss) {
		text403(w, errOnlyRegistered)
		return false
	}
	return true
}

// Ensure only registered users can post.
func assertNotWhitelistOnlyAPI(w http.ResponseWriter, board string, ss *auth.Session) bool {
	if !checkWhitelistOnly(board, ss) {
		text403(w, errOnlyWhitelist)
		return false
	}
	return true
}

// Ensure user not blacklisted.
func assertNotBlacklisted(w http.ResponseWriter, board string, ss *auth.Session) bool {
	if checkPosition(board, ss, 1) {
		text403(w, errBannedBoard)
		return false
	}
	return true
}

func checkModOnly(board string, ss *auth.Session) bool {
	if !config.IsModOnlyBoard(board) {
		return true
	}
	if ss == nil {
		return false
	}
	return ss.Positions.CurBoard >= auth.Moderator
}

// Ensure only mods and above can view mod-only boards.
func assertNotModOnly(w http.ResponseWriter, r *http.Request, board string, ss *auth.Session) bool {
	if !checkModOnly(board, ss) {
		serve404(w, r)
		return false
	}
	return true
}

// Ensure only mods and above can post at mod-only boards.
func assertNotModOnlyAPI(w http.ResponseWriter, board string, ss *auth.Session) bool {
	if !checkModOnly(board, ss) {
		text400(w, errInvalidBoard)
		return false
	}
	return true
}

func checkPowerUser(ss *auth.Session) bool {
	if ss == nil {
		return false
	}
	return ss.Positions.IsPowerUser()
}

// Ensure only power users can pass.
func assertPowerUserAPI(w http.ResponseWriter, ss *auth.Session) bool {
	if !checkPowerUser(ss) {
		text403(w, aerrPowerUserOnly)
		return false
	}
	return true
}

// Calculate and check provided ETag by simply hashing the content.
// This doesn't make page generation faster, only saves network traffic.
func assertCached(
	w http.ResponseWriter,
	r *http.Request,
	buf []byte,
) bool {
	etag := fmt.Sprintf("W/\"%s\"", util.HashBuffer(buf))
	// https://tools.ietf.org/html/rfc2616#section-10.3.5
	// https://stackoverflow.com/a/4226409
	w.Header().Set("ETag", etag)
	if etag == r.Header.Get("If-None-Match") {
		w.WriteHeader(304)
		return true
	}
	return false
}

type AdminBoardHandler func(
	w http.ResponseWriter,
	r *http.Request,
	ss *auth.Session,
	board string,
)

func assertBoardOwner(h AdminBoardHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ss, _ := getSession(r, "")
		if ss == nil || ss.Positions.AnyBoard < auth.BoardOwner {
			text403(w, aerrBoardOwnersOnly)
			return
		}
		h(w, r, ss, "")
	}
}

type AdminBoardAPIHandler func(r *http.Request, ss *auth.Session, board string) error

func assertBoardOwnerAPI(h AdminBoardAPIHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		board := getParam(r, "board")
		if !assertBoardAPI(w, board) {
			return
		}
		ss, _ := getSession(r, board)
		if ss == nil || ss.Positions.CurBoard < auth.BoardOwner {
			serveErrorJSON(w, r, aerrBoardOwnersOnly)
			return
		}
		err := h(r, ss, board)
		if err != nil {
			serveErrorJSON(w, r, err)
			return
		}
		serveEmptyJSON(w, r)
	}
}
