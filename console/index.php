<?php
$url = array(
	'/api/v1/app'=>'app',
	'/api/v1/message'=>'message',
);
$uri = $_SERVER['uri'];
isset($url[$uri]) and $url[$uri]();

function app() {
	echo  file_get_connect("http://push.scloud.letv.com/api/v1/app?" . http_build_query($_GET));
}

function message() {
	$post = file_get_contents('php://input');
	$curl = new LCurl();
	$curl->init();
	$curl->da
}

/**
 * need php 5.5
 */
class LCurl {

    /**
     *
     * @var string 
     */
    public $url;

    /**
     *
     * @var string 
     */
    public $ref;

    /**
     *
     * @var int
     */
    public $errno;

    /**
     *
     * @var string 
     */
    public $error;

    /**
     *
     * @var array 
     */
    public $info;

    /**
     *
     * @var int 
     */
    public $status;

    /**
     *
     * @var string 
     */
    public $body;

    /**
     *
     * @var array 
     */
    public $data;

    /**
     *
     * @var array 
     */
    public $params;

    /**
     *
     * @var bool 
     */
    public $json_body;

    /**
     *
     * @var int 
     */
    public $json_errno;

    /**
     *
     * @var string 
     */
    public $json_error;

    /**
     *
     * @var bool 
     */
    public $ajax;

    public function __construct() {
        $this->init();
    }

    public function init() {
        $this->init_param();
        $this->url = false;
        $this->ref = false;
        $this->errno = 0;
        $this->error = '';
        $this->info = array();
        $this->status = 0;
        $this->body = false;
        $this->data = array();
        $this->json_body = false;
        $this->json_errno = JSON_ERROR_NONE;
        $this->json_error = null;
        $this->ajax = false;
    }

    /**
     * 设置请求data
     * @param string $key
     * @param string $value
     * @return maxed
     */
    public function data($key, $value = false) {
        if (false !== $value) {
            $this->data[$key] = $value;
        }
        return isset($this->data[$key]) ? $this->data[$key] : false;
    }

    /**
     * 结果json返回
     * @param bool $direct 直接返回结果
     * @param bool $obj 返回对象
     * @return \stdClass
     */
    private function body_json() {
        if (false === $this->json_body) {
            return true;
        }
        $this->body = json_decode($this->body,1);
        $this->json_errno = json_last_error();
        $this->json_error = '';//json_last_error_msg();

        return JSON_ERROR_NONE === $this->json_errno;
    }

    private function init_param() {
        $this->params = array();
        /**
         * 在发起连接前等待的时间，如果设置为0，则无限等待。
         */
        $this->params[CURLOPT_CONNECTTIMEOUT] = 10;
        /**
         * 设置cURL允许执行的最长秒数。
         */
        $this->params[CURLOPT_TIMEOUT] = 30;
        /**
         * HTTP请求头中"Accept-Encoding: "的值。
         * 支持的编码有"identity"，"deflate"和"gzip"。
         * 如果为空字符串""，请求头会发送所有支持的编码类型。
         */
        $this->params[CURLOPT_ENCODING] = '';
        /**
         * 在HTTP请求中包含一个"User-Agent: "头的字符串。
         */
        $this->params[CURLOPT_USERAGENT] = 'Mozilla/5.0 (compatible; MSIE 10.0; Windows NT 6.1; Trident/6.0)';
        /**
         * 启用时会将服务器服务器返回的"Location:
         * "放在header中递归的返回给服务器，使用CURLOPT_MAXREDIRS可以限定递归返回的数量。
         */
        $this->params[CURLOPT_FOLLOWLOCATION] = true;

        /**
         * 指定最多的HTTP重定向的数量，这个选项是和CURLOPT_FOLLOWLOCATION一起使用的。
         */
        $this->params[CURLOPT_MAXREDIRS] = 5;

        /**
         * 一个用来设置HTTP头字段的数组。使用如下的形式的数组进行设置：
         * array('Content-type: text/plain', 'Content-length: 100')
         */
        $this->params[CURLOPT_HTTPHEADER] = array(
            'HTTP_ACCEPT: */*',
        );
        /**
         * 将 curl_exec()获取的信息以文件流的形式返回，而不是直接输出。
         */
        $this->params[CURLOPT_RETURNTRANSFER] = true;
    }

    /**
     * 
     * @param int $key
     * @param mixed $value
     * @return \Curl
     */
    public function params($key, $value = null) {
        if (null === $value) {
            return isset($this->params[$key]) ? $this->params[$key] : null;
        }
        $this->params[$key] = $value;

        return $this;
    }

    /**
     * 
     * @return string
     */
    public function request() {
        $ch = curl_init();
        $this->ref & $this->params[CURLOPT_REFERER] = $this->ref;
        if (!empty($this->data)) {
            if (!isset($this->params[CURLOPT_CUSTOMREQUEST]) || 'GET' == $this->params[CURLOPT_CUSTOMREQUEST]) {
                $this->url .= false === strpos($this->url, '?') ? '?' : '&';
                $this->url .= http_build_query($this->data);
            } else {
                $this->params[CURLOPT_POSTFIELDS] = $this->data;
            }
        }
        if ($this->ajax) {
            $this->params[CURLOPT_HTTPHEADER][] = 'X-Requested-With: XMLHttpRequest';
        }
        $this->params[CURLOPT_URL] = $this->url;
        curl_setopt_array($ch, $this->params);

        $this->body = curl_exec($ch);
        $this->errno = curl_errno($ch);
        $this->error = curl_error($ch);
        $this->info = curl_getinfo($ch);
        $this->status = $this->info['http_code'];
        curl_close($ch);
        $this->body_json();

        return $this->body;
    }

    /**
     * 
     * @return string
     */
    public function get() {
        $this->params[CURLOPT_CUSTOMREQUEST] = 'GET';
        return $this->request();
    }

    public function post() {
        $this->params[CURLOPT_CUSTOMREQUEST] = 'POST';
        $this->request();
    }

    public function delete() {
        $this->params[CURLOPT_CUSTOMREQUEST] = 'DELETE';
        $this->request();
    }

    public function put() {
        $this->params[CURLOPT_CUSTOMREQUEST] = 'PUT';
        $this->request();
    }

    //set params start
    /**
     * 设置ua
     * @param string $ua
     * @return \Curl
     */
    public function userAgent($ua) {
        $this->params[CURLOPT_USERAGENT] = $ua;
        return $this;
    }

}

?>
