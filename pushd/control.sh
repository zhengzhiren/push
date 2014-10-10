#!/bin/bash

cmd=$1

THIS_DIR=$(dirname $(readlink -f $0) )

function start()
{
	sleep 3
	supervisord -c $THIS_DIR/conf/supervisord.conf
	[ $? -ne 0 ] && { echo "start supervisord failed"; exit 1; }
	retry=0
	while [ $retry -lt 5 ]; do
		supervisorctl -c $THIS_DIR/conf/supervisord.conf status pushd |grep RUNNING >/dev/null
		[ $? -eq 0 ] && { break; }
		retry=$(($retry+1))
		sleep 1
	done
	[ $? -ge 5 ] && { echo "push server not in running status"; return 1; }
	return 0
}

function stop()
{
	supervisorctl -c $THIS_DIR/conf/supervisord.conf shutdown >/dev/null 2>&1
	return 0
	#ps axf|grep supervisord |grep push-server
	
}

function restart()
{
	stop
	start
}

case $cmd in
	start)
		start
		;;
	stop)
		stop
		;;	
	restart)
		restart
		;;
	*)
		echo $"Usage: $0 {start|stop|restart}"
		RET=2	
esac
exit $RET

