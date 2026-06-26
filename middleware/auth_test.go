package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserAuthRefreshesGroupFromUserCache(t *testing.T) {
	previousDB := model.DB
	previousRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	common.RedisEnabled = false

	require.NoError(t, db.Create(&model.User{
		Id:       10,
		Username: "tester",
		Status:   common.UserStatusEnabled,
		Group:    "ChatGPTPro",
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("auth-refresh-test"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "tester")
		session.Set("role", common.RoleCommonUser)
		session.Set("id", 10)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "ChatGPT")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	router.GET("/protected", UserAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"group":      c.GetString("group"),
			"user_group": c.GetString("user_group"),
		})
	})

	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("New-Api-User", "10")
	for _, c := range loginRecorder.Result().Cookies() {
		request.AddCookie(c)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"group":"ChatGPTPro","user_group":"ChatGPTPro"}`, recorder.Body.String())
}
