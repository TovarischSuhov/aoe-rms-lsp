// rules.xs — rules with modifiers, condition/action sections
rule resource_check inactive min-interval 10 {
	condition {
		kbResourceGet(1, 2) > 5
	}
	action {
		xsSetVar(1, 2);
	}
}

rule always {
	condition {
		true
	}
	action {
		xsChatHistory(1, "hi")
	}
}
