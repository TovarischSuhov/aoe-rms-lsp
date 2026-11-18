// c46-lead
rule always {
condition { }
action { }
}

rule permissive {
/* c46-before-condition */
condition: kbResourceGet(1, 2) > 5;
action {
xsSetVar(1, 2); // c46-in-action
}
action: xsChatHistory(1, "direct");
}
