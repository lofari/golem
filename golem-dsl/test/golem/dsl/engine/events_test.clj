(ns golem.dsl.engine.events-test
  (:require [clojure.test :refer :all]
            [clojure.data.json :as json]
            [golem.dsl.engine.events :as events]))

(deftest emit-step-start-test
  (let [output (with-out-str (events/emit! :step-start {:step "plan" :iteration 1 :agent "build-feature"}))]
    (is (= "step-start" (get (json/read-str output) "type")))
    (is (= "plan" (get (json/read-str output) "step")))))

(deftest emit-agent-done-test
  (let [output (with-out-str (events/emit! :agent-done {:agent "build-feature" :outcome "complete" :total-steps 5}))]
    (is (= "agent-done" (get (json/read-str output) "type")))
    (is (= "complete" (get (json/read-str output) "outcome")))))

(deftest emit-error-test
  (let [output (with-out-str (events/emit! :error {:step "implement" :error-type "timeout" :iteration 2}))]
    (is (= "error" (get (json/read-str output) "type")))
    (is (= "timeout" (get (json/read-str output) "error-type")))))
