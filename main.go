package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/oringik/crypto-chateau/gen/defs"
	"github.com/oringik/crypto-chateau/message"
	"github.com/oringik/crypto-chateau/server"
	"io"
	"io/ioutil"
	"log"
	"os"
	"plugin"
	"reflect"
	"strconv"
	"strings"
)

var (
	inputFilepath  string
	inputArgs      string
	connString     string
	endpointString string
)

func init() {
	flag.StringVar(&connString, "conn_string", "", "conn string")
	flag.StringVar(&inputFilepath, "chateau_file", "", "chateau file")
	flag.StringVar(&endpointString, "endpoint", "", "endpoint for calling")
	flag.StringVar(&inputArgs, "args", "", "request args")
}

func main() {
	flag.Parse()

	ctx := context.Background()

	err := copyFile(inputFilepath, "./copy.chateau")
	if err != nil {
		log.Fatalln("error copying file: ", err.Error())
	}

	err = changePackageName(inputFilepath)
	if err != nil {
		log.Fatalln("error changing package name: " + err.Error())
	}

	err = defs.GenerateDefinitions(inputFilepath, "./generated", "go")
	if err != nil {
		log.Fatalln(err)
	}

	pl, err := plugin.Open("./generated/gen_definitions.go")
	if err != nil {
		log.Fatalln("error oppening plugin: " + err.Error())
	}

	conns := strings.Split(connString, ":")
	if len(conns) != 2 {
		log.Fatalln("incorrect conn string")
	}

	endpoint := strings.Split(endpointString, ".")

	getHandlersFunc, err := pl.Lookup("GenEmptyHandlers")
	if err != nil {
		log.Fatalln(err)
	}

	handlers := getHandlersFunc.(func() map[string]*server.Handler)()

	handler, ok := handlers[endpoint[1]]
	if !ok {
		log.Fatalln("incorrect method name")
	}

	var args map[string]string
	err = json.Unmarshal([]byte(inputArgs), &args)
	if err != nil {
		log.Fatalf("incorrect input args: " + err.Error())
	}

	req := handler.RequestMsgType

	err = fillReq(req, args)
	if err != nil {
		log.Fatalf("error filling args to req object: " + err.Error())
	}

	port, err := strconv.Atoi(conns[1])
	if err != nil {
		log.Fatalf("incorrect port: " + err.Error())
	}

	callClientMethodFunc, err := pl.Lookup("CallClientMethod")
	if err != nil {
		log.Fatalln(err)
	}

	response, err := callClientMethodFunc.(func(context.Context, string, int, string, string, message.Message) (message.Message, error))(ctx, conns[0], port, endpoint[0], endpoint[1], req)
	if err != nil {
		log.Fatalf("error calling client method: " + err.Error())
	}

	jsonResp, err := json.Marshal(response)
	if err != nil {
		log.Fatalln("error marshalling response to json: " + err.Error())
	}

	log.Println(string(jsonResp))
}

func fillReq(req message.Message, params map[string]string) error {
	v := reflect.ValueOf(req)
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return errors.New("require pointer to a struct")
	}

	return setFields(v, params)
}

func setFields(v reflect.Value, params map[string]string) error {
	typ := v.Type()

	for fieldNum := 0; fieldNum < typ.NumField(); fieldNum++ {
		field := typ.Field(fieldNum)
		value := params[field.Name]

		switch field.Type.Kind() {
		case reflect.String:
			v.Field(fieldNum).SetString(value)
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint8, reflect.Uint32, reflect.Uint64:
			num, err := strconv.Atoi(value)
			if err != nil {
				return errors.New("error converting param to int: " + err.Error())
			}

			v.Field(fieldNum).SetInt(int64(num))
		case reflect.Float32, reflect.Float64:
			var bitSize int
			if field.Type.Kind() == reflect.Float32 {
				bitSize = 32
			} else {
				bitSize = 64
			}

			num, err := strconv.ParseFloat(value, bitSize)
			if err != nil {
				return errors.New("error converting param to float32: " + err.Error())
			}

			v.Field(fieldNum).SetFloat(num)
		case reflect.Bool:
			var b bool
			if value == "true" {
				b = true
			} else {
				b = false
			}

			v.Field(fieldNum).SetBool(b)
		case reflect.Ptr, reflect.Struct:

			var internalV reflect.Value
			if v.Field(fieldNum).Type().Kind() == reflect.Ptr {
				internalV = v.Field(fieldNum).Elem()
			} else {
				internalV = v
			}

			var internalValues map[string]string
			err := json.Unmarshal([]byte(value), &internalValues)
			if err != nil {
				return err
			}

			err = setFields(internalV, internalValues)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(destination, source)
	return err
}

func changePackageName(inputFile string) error {
	input, err := ioutil.ReadFile(inputFile)
	if err != nil {
		log.Fatalln(err)
	}

	lines := strings.Split(string(input), "\n")
	lines[0] = "package generated"

	output := strings.Join(lines, "\n")
	err = ioutil.WriteFile(inputFile, []byte(output), 0644)
	if err != nil {
		log.Fatalln(err)
	}

	return nil
}
