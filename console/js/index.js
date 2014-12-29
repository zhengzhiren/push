$(function() {
	// $('#getpkg').on('click')
	var Go = function(opt) {
		if (!opt.data) {
			opt.data = {};
		}
		var data = opt.data;
		if (!(opt.type == "post")) { //get请求加时间戳
			var t = new Date().getTime();
			if (typeof(opt.data) == 'string') {
				opt.data = opt.data + '&t=' + t; //字符串参数
			} else {
				$.extend(opt.data, {
					t: t
				})
			}
		}
		$.ajax(opt);
	}

	var getAppid = function() {
		var form = $('#getpkg');
		form.on('submit', function(e) {
			e.preventDefault();
			var data = $(this).serialize();
			var option = {
				url: 'http://push.scloud.letv.com/api/v1/app',
				dataType: 'json',
				type: 'get',
				data: data,
				success: function(json) {
					form.find('.l-text-line').html(json.data.appid);
					var input = $("#pushform").find("[name=appid]");
					if(input.length < 1){
						var input = $("<input type='hidden' name='appid' />");
						input.val(json.data.appid);
						$('#pushform').append(input);
					} else {
						input.val(json.data.appid);
					}
				},
 				fail: function(err) {
					console.log(err);
				}
			}
			new Go(option);
		})
	}
	var Push = function() {
		this.element = $('#pushform');
		this.init();
	}
	$.extend(Push.prototype, {
		init: function() {
			this.bind();
		},
		bind: function() {
			var me = this;
			var radios = this.element.find('[name="msg_type"]');
			radios.on('change', function(e) {
				e.preventDefault();
				var checked = $(this).prop('checked');
				var id = $(this).attr('id');
				if( (id == 'inlineRadio1') && checked) {
					me.hideMessageTextarea();
				} else if(checked){
					me.showMessageTextarea();
				}
			
			}).change();
			this.element.on('submit', function(e) {
				e.preventDefault();
				 var data = $(this).serialize();
				 var newdata = data.split("&");
				 var temp = {};
				 for(var i = 0; i<newdata.length; i++) {
					var arr = newdata[i].split('=');
					if(arr[0] == "msg_type" || arr[0] == "push_type") {
						temp[arr[0]] = parseInt(arr[1]);
					} else {
						temp[arr[0]] = arr[1];
					}
				 }
				 console.log(temp);
				 var appid = $(this).find('[name=appid]').val();
				 var option = {
				 	url: 'http://push.scloud.letv.com/api/v1/message',
				 	dataType: 'json',
				 	type: 'post',
				 	data: JSON.stringify(temp),
					headers : {
						"Authorization" : "LETV "+appid +" pushtest"
					},
				 	success: function(json) {
				 		//form.find('.l-text-line').html(json);
				 		$('#result').val(json)
				 		$('#result').closest('.row').show();
				 	}
				 }
				 new Go(option);
			//	$('#result').val('插入内容');
			//	$('#result').closest('.row').show();
				
			})
		},
		showMessageTextarea: function() {
			var notice = this.element.find('.notice');
			var textarea = this.element.find('#messageContent');
			textarea.show();
			notice.hide();
		},
		hideMessageTextarea: function() {
			var notice = this.element.find('.notice');
			var textarea = this.element.find('#messageContent');
			textarea.hide();
			notice.show();
		}
	});
	new Push();
	getAppid();
})
