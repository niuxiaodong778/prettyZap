package prettyZap

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	DefaultPort         = "9090"
	DefaultLevel        = "info"
	DefaultURL          = "/change/level"
	DefaultMaxLogSizeMb = 256
	DefaultMaxBackup    = 10
	DefaultMaxAgeDay    = 7
	DefaultSvcName      = "app"
	IsCompress          = false
)

const (
	LogOutputStdout        = iota // 0
	LogOutputFile                 // 1
	LogOutputStdoutAndFile        // 2
)

type PreSetConfig struct {
	LogFilePath  string
	HttpPort     string
	LogLevel     string
	RestURL      string
	MaxLogSizeMb int
	MaxBackup    int
	MaxAgeDay    int
	SvcName      string
	IsCompress   bool
	LogOutputTo  int
}

var zapLogger *zap.SugaredLogger
var atomicLevel = zap.NewAtomicLevel()

var levelMap = map[string]zapcore.Level{
	"debug":  zapcore.DebugLevel,
	"info":   zapcore.InfoLevel,
	"warn":   zapcore.WarnLevel,
	"error":  zapcore.ErrorLevel,
	"dpanic": zapcore.DPanicLevel,
	"panic":  zapcore.PanicLevel,
	"fatal":  zapcore.FatalLevel,
}

var DefaultCfg = PreSetConfig{
	LogFilePath:  getFilePath(),
	HttpPort:     DefaultPort,
	LogLevel:     DefaultLevel,
	RestURL:      DefaultURL,
	MaxLogSizeMb: DefaultMaxLogSizeMb,
	MaxBackup:    DefaultMaxBackup,
	MaxAgeDay:    DefaultMaxAgeDay,
	SvcName:      getAppname(),
	IsCompress:   IsCompress,
	LogOutputTo:  LogOutputStdoutAndFile,
}

var encoderConfig = zapcore.EncoderConfig{
	TimeKey:        "time",
	LevelKey:       "level",
	NameKey:        "zapLogger",
	CallerKey:      "caller",
	MessageKey:     "msg",
	StacktraceKey:  "stacktrace",
	LineEnding:     zapcore.DefaultLineEnding,
	EncodeLevel:    zapcore.LowercaseLevelEncoder,  // 小写编码器
	EncodeTime:     zapcore.ISO8601TimeEncoder,     // ISO8601 UTC 时间格式
	EncodeDuration: zapcore.SecondsDurationEncoder, //
	EncodeCaller:   zapcore.ShortCallerEncoder,     // 短路径编码器
	// EncodeCaller:   zapcore.FullCallerEncoder,    // 全路径编码器
	EncodeName: zapcore.FullNameEncoder,
}

func getLoggerLevel(lvl string) zapcore.Level {
	if level, ok := levelMap[lvl]; ok {
		return level
	}
	return zapcore.InfoLevel
}

func InitPrettyZap(preCfg *PreSetConfig) {
	transferCfg(preCfg, &DefaultCfg)
	http.HandleFunc(DefaultCfg.RestURL, atomicLevel.ServeHTTP)
	go func() {
		if err := http.ListenAndServe(":"+DefaultCfg.HttpPort, nil); err != nil {
			panic(err)
		}
	}()

	log := NewLogger(&DefaultCfg)
	// defer log.Sync()
	zapLogger = log.Sugar()
	zapLogger.Sync()
	// SugaredLogger transfer back to Logger object
	// plain := zapLogger.Desugar()
}

func transferCfg(preConfig, runCfg *PreSetConfig) {
	if preConfig != nil && runCfg != nil {
		if runCfg.IsCompress != preConfig.IsCompress {
			runCfg.IsCompress = preConfig.IsCompress
		}
		if runCfg.MaxAgeDay != preConfig.MaxAgeDay {
			runCfg.MaxAgeDay = preConfig.MaxAgeDay
		}
		if runCfg.SvcName != preConfig.SvcName {
			runCfg.SvcName = preConfig.SvcName
		}
		if runCfg.MaxBackup != preConfig.MaxBackup {
			runCfg.MaxBackup = preConfig.MaxBackup
		}
		if runCfg.LogLevel != preConfig.LogLevel {
			runCfg.LogLevel = preConfig.LogLevel
		}
		if runCfg.RestURL != preConfig.RestURL {
			runCfg.RestURL = preConfig.RestURL
		}
		if runCfg.HttpPort != preConfig.HttpPort {
			runCfg.HttpPort = preConfig.HttpPort
		}
		if runCfg.MaxLogSizeMb != preConfig.MaxLogSizeMb {
			runCfg.MaxLogSizeMb = preConfig.MaxLogSizeMb
		}
		if runCfg.LogFilePath != preConfig.LogFilePath {
			runCfg.LogFilePath = preConfig.LogFilePath
		}
		if runCfg.LogOutputTo != preConfig.LogOutputTo {
			runCfg.LogOutputTo = preConfig.LogOutputTo
		}
	}
}

func NewLogger(cfg *PreSetConfig) *zap.Logger {
	return zap.New(newCore(cfg),
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.Development(),
		zap.Fields(zap.String("serviceName", cfg.SvcName)))
}

func outputTo(cfg *PreSetConfig) []zapcore.WriteSyncer {
	var multiWriteSyncer []zapcore.WriteSyncer
	hook := lumberjack.Logger{
		Filename:   cfg.LogFilePath,  // 日志文件路径
		MaxSize:    cfg.MaxLogSizeMb, // 每个日志文件保存的最大尺寸 单位：M
		MaxBackups: cfg.MaxBackup,    // 日志文件最多保存多少个备份
		MaxAge:     cfg.MaxAgeDay,    // 文件最多保存多少天
		Compress:   cfg.IsCompress,   // 是否压缩
	}
	switch cfg.LogOutputTo {
	case LogOutputStdout:
		multiWriteSyncer = append(multiWriteSyncer, zapcore.AddSync(os.Stdout))
		break
	case LogOutputFile:
		multiWriteSyncer = append(multiWriteSyncer, zapcore.AddSync(&hook))
		break
	default:
		multiWriteSyncer = append(multiWriteSyncer, zapcore.AddSync(os.Stdout), zapcore.AddSync(&hook))
	}
	return multiWriteSyncer
}

func newCore(cfg *PreSetConfig) zapcore.Core {
	multiWriteSyncer := outputTo(cfg)
	return zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),            // 编码器配置
		zapcore.NewMultiWriteSyncer(multiWriteSyncer...), // 打印到控制台和文件
		getLoggerLevel(DefaultCfg.LogLevel),              // 日志级别
	)
}

func getCurrentDirectory() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		zapLogger.Info(err)
	}
	return dir
}

func getFilePath() string {
	logfile := getCurrentDirectory() + "/" + getAppname() + ".log"
	return logfile
}

func getAppname() string {
	full := os.Args[0]
	splits := strings.Split(full, "/")
	if len(splits) >= 1 {
		name := splits[len(splits)-1]
		return name
	}
	return DefaultSvcName
}

func Debug(format interface{}, args ...interface{}) {
	switch templet := format.(type) {
	case string:
		zapLogger.Debugf(templet, args...)
	default:
		zapLogger.Debugf(fmt.Sprint(format)+strings.Repeat(" %v", len(args)), args...)
	}
}

func Info(format interface{}, args ...interface{}) {
	switch templet := format.(type) {
	case string:
		zapLogger.Infof(templet, args...)
	default:
		zapLogger.Infof(fmt.Sprint(format)+strings.Repeat(" %v", len(args)), args...)
	}
}

func Warn(format interface{}, args ...interface{}) {
	switch templet := format.(type) {
	case string:
		zapLogger.Warnf(templet, args...)
	default:
		zapLogger.Warnf(fmt.Sprint(format)+strings.Repeat(" %v", len(args)), args...)
	}
}

func Error(format interface{}, args ...interface{}) {
	switch templet := format.(type) {
	case string:
		zapLogger.Errorf(templet, args...)
	default:
		zapLogger.Errorf(fmt.Sprint(format)+strings.Repeat(" %v", len(args)), args...)
	}
}

func Panic(format interface{}, args ...interface{}) {
	switch templet := format.(type) {
	case string:
		zapLogger.Panicf(templet, args...)
	default:
		zapLogger.Panicf(fmt.Sprint(format)+strings.Repeat(" %v", len(args)), args...)
	}
}
