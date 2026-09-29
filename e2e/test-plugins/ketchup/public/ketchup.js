/**
 * @license
 * Copyright 2019 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const se=globalThis,Le=se.ShadowRoot&&(se.ShadyCSS===void 0||se.ShadyCSS.nativeShadow)&&"adoptedStyleSheets"in Document.prototype&&"replace"in CSSStyleSheet.prototype,ze=Symbol(),Ve=new WeakMap;let Ss=class{constructor(t,s,i){if(this._$cssResult$=!0,i!==ze)throw Error("CSSResult is not constructable. Use `unsafeCSS` or `css` instead.");this.cssText=t,this.t=s}get styleSheet(){let t=this.o;const s=this.t;if(Le&&t===void 0){const i=s!==void 0&&s.length===1;i&&(t=Ve.get(s)),t===void 0&&((this.o=t=new CSSStyleSheet).replaceSync(this.cssText),i&&Ve.set(s,t))}return t}toString(){return this.cssText}};const Ks=e=>new Ss(typeof e=="string"?e:e+"",void 0,ze),yt=(e,...t)=>{const s=e.length===1?e[0]:t.reduce((i,a,o)=>i+(r=>{if(r._$cssResult$===!0)return r.cssText;if(typeof r=="number")return r;throw Error("Value passed to 'css' function must be a 'css' function result: "+r+". Use 'unsafeCSS' to pass non-literal values, but take care to ensure page security.")})(a)+e[o+1],e[0]);return new Ss(s,e,ze)},Gs=(e,t)=>{if(Le)e.adoptedStyleSheets=t.map(s=>s instanceof CSSStyleSheet?s:s.styleSheet);else for(const s of t){const i=document.createElement("style"),a=se.litNonce;a!==void 0&&i.setAttribute("nonce",a),i.textContent=s.cssText,e.appendChild(i)}},qe=Le?e=>e:e=>e instanceof CSSStyleSheet?(t=>{let s="";for(const i of t.cssRules)s+=i.cssText;return Ks(s)})(e):e;/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const{is:Js,defineProperty:Qs,getOwnPropertyDescriptor:ti,getOwnPropertyNames:ei,getOwnPropertySymbols:si,getPrototypeOf:ii}=Object,pe=globalThis,Ze=pe.trustedTypes,ai=Ze?Ze.emptyScript:"",oi=pe.reactiveElementPolyfillSupport,Bt=(e,t)=>e,ne={toAttribute(e,t){switch(t){case Boolean:e=e?ai:null;break;case Object:case Array:e=e==null?e:JSON.stringify(e)}return e},fromAttribute(e,t){let s=e;switch(t){case Boolean:s=e!==null;break;case Number:s=e===null?null:Number(e);break;case Object:case Array:try{s=JSON.parse(e)}catch{s=null}}return s}},Ae=(e,t)=>!Js(e,t),Ke={attribute:!0,type:String,converter:ne,reflect:!1,useDefault:!1,hasChanged:Ae};Symbol.metadata??=Symbol("metadata"),pe.litPropertyMetadata??=new WeakMap;let It=class extends HTMLElement{static addInitializer(t){this._$Ei(),(this.l??=[]).push(t)}static get observedAttributes(){return this.finalize(),this._$Eh&&[...this._$Eh.keys()]}static createProperty(t,s=Ke){if(s.state&&(s.attribute=!1),this._$Ei(),this.prototype.hasOwnProperty(t)&&((s=Object.create(s)).wrapped=!0),this.elementProperties.set(t,s),!s.noAccessor){const i=Symbol(),a=this.getPropertyDescriptor(t,i,s);a!==void 0&&Qs(this.prototype,t,a)}}static getPropertyDescriptor(t,s,i){const{get:a,set:o}=ti(this.prototype,t)??{get(){return this[s]},set(r){this[s]=r}};return{get:a,set(r){const n=a?.call(this);o?.call(this,r),this.requestUpdate(t,n,i)},configurable:!0,enumerable:!0}}static getPropertyOptions(t){return this.elementProperties.get(t)??Ke}static _$Ei(){if(this.hasOwnProperty(Bt("elementProperties")))return;const t=ii(this);t.finalize(),t.l!==void 0&&(this.l=[...t.l]),this.elementProperties=new Map(t.elementProperties)}static finalize(){if(this.hasOwnProperty(Bt("finalized")))return;if(this.finalized=!0,this._$Ei(),this.hasOwnProperty(Bt("properties"))){const s=this.properties,i=[...ei(s),...si(s)];for(const a of i)this.createProperty(a,s[a])}const t=this[Symbol.metadata];if(t!==null){const s=litPropertyMetadata.get(t);if(s!==void 0)for(const[i,a]of s)this.elementProperties.set(i,a)}this._$Eh=new Map;for(const[s,i]of this.elementProperties){const a=this._$Eu(s,i);a!==void 0&&this._$Eh.set(a,s)}this.elementStyles=this.finalizeStyles(this.styles)}static finalizeStyles(t){const s=[];if(Array.isArray(t)){const i=new Set(t.flat(1/0).reverse());for(const a of i)s.unshift(qe(a))}else t!==void 0&&s.push(qe(t));return s}static _$Eu(t,s){const i=s.attribute;return i===!1?void 0:typeof i=="string"?i:typeof t=="string"?t.toLowerCase():void 0}constructor(){super(),this._$Ep=void 0,this.isUpdatePending=!1,this.hasUpdated=!1,this._$Em=null,this._$Ev()}_$Ev(){this._$ES=new Promise(t=>this.enableUpdating=t),this._$AL=new Map,this._$E_(),this.requestUpdate(),this.constructor.l?.forEach(t=>t(this))}addController(t){(this._$EO??=new Set).add(t),this.renderRoot!==void 0&&this.isConnected&&t.hostConnected?.()}removeController(t){this._$EO?.delete(t)}_$E_(){const t=new Map,s=this.constructor.elementProperties;for(const i of s.keys())this.hasOwnProperty(i)&&(t.set(i,this[i]),delete this[i]);t.size>0&&(this._$Ep=t)}createRenderRoot(){const t=this.shadowRoot??this.attachShadow(this.constructor.shadowRootOptions);return Gs(t,this.constructor.elementStyles),t}connectedCallback(){this.renderRoot??=this.createRenderRoot(),this.enableUpdating(!0),this._$EO?.forEach(t=>t.hostConnected?.())}enableUpdating(t){}disconnectedCallback(){this._$EO?.forEach(t=>t.hostDisconnected?.())}attributeChangedCallback(t,s,i){this._$AK(t,i)}_$ET(t,s){const i=this.constructor.elementProperties.get(t),a=this.constructor._$Eu(t,i);if(a!==void 0&&i.reflect===!0){const o=(i.converter?.toAttribute!==void 0?i.converter:ne).toAttribute(s,i.type);this._$Em=t,o==null?this.removeAttribute(a):this.setAttribute(a,o),this._$Em=null}}_$AK(t,s){const i=this.constructor,a=i._$Eh.get(t);if(a!==void 0&&this._$Em!==a){const o=i.getPropertyOptions(a),r=typeof o.converter=="function"?{fromAttribute:o.converter}:o.converter?.fromAttribute!==void 0?o.converter:ne;this._$Em=a;const n=r.fromAttribute(s,o.type);this[a]=n??this._$Ej?.get(a)??n,this._$Em=null}}requestUpdate(t,s,i,a=!1,o){if(t!==void 0){const r=this.constructor;if(a===!1&&(o=this[t]),i??=r.getPropertyOptions(t),!((i.hasChanged??Ae)(o,s)||i.useDefault&&i.reflect&&o===this._$Ej?.get(t)&&!this.hasAttribute(r._$Eu(t,i))))return;this.C(t,s,i)}this.isUpdatePending===!1&&(this._$ES=this._$EP())}C(t,s,{useDefault:i,reflect:a,wrapped:o},r){i&&!(this._$Ej??=new Map).has(t)&&(this._$Ej.set(t,r??s??this[t]),o!==!0||r!==void 0)||(this._$AL.has(t)||(this.hasUpdated||i||(s=void 0),this._$AL.set(t,s)),a===!0&&this._$Em!==t&&(this._$Eq??=new Set).add(t))}async _$EP(){this.isUpdatePending=!0;try{await this._$ES}catch(s){Promise.reject(s)}const t=this.scheduleUpdate();return t!=null&&await t,!this.isUpdatePending}scheduleUpdate(){return this.performUpdate()}performUpdate(){if(!this.isUpdatePending)return;if(!this.hasUpdated){if(this.renderRoot??=this.createRenderRoot(),this._$Ep){for(const[a,o]of this._$Ep)this[a]=o;this._$Ep=void 0}const i=this.constructor.elementProperties;if(i.size>0)for(const[a,o]of i){const{wrapped:r}=o,n=this[a];r!==!0||this._$AL.has(a)||n===void 0||this.C(a,void 0,o,n)}}let t=!1;const s=this._$AL;try{t=this.shouldUpdate(s),t?(this.willUpdate(s),this._$EO?.forEach(i=>i.hostUpdate?.()),this.update(s)):this._$EM()}catch(i){throw t=!1,this._$EM(),i}t&&this._$AE(s)}willUpdate(t){}_$AE(t){this._$EO?.forEach(s=>s.hostUpdated?.()),this.hasUpdated||(this.hasUpdated=!0,this.firstUpdated(t)),this.updated(t)}_$EM(){this._$AL=new Map,this.isUpdatePending=!1}get updateComplete(){return this.getUpdateComplete()}getUpdateComplete(){return this._$ES}shouldUpdate(t){return!0}update(t){this._$Eq&&=this._$Eq.forEach(s=>this._$ET(s,this[s])),this._$EM()}updated(t){}firstUpdated(t){}};It.elementStyles=[],It.shadowRootOptions={mode:"open"},It[Bt("elementProperties")]=new Map,It[Bt("finalized")]=new Map,oi?.({ReactiveElement:It}),(pe.reactiveElementVersions??=[]).push("2.1.2");/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const je=globalThis,Ge=e=>e,ce=je.trustedTypes,Je=ce?ce.createPolicy("lit-html",{createHTML:e=>e}):void 0,Ps="$lit$",nt=`lit$${Math.random().toFixed(9).slice(2)}$`,$s="?"+nt,ri=`<${$s}>`,bt=document,Ut=()=>bt.createComment(""),Wt=e=>e===null||typeof e!="object"&&typeof e!="function",Oe=Array.isArray,ni=e=>Oe(e)||typeof e?.[Symbol.iterator]=="function",be=`[ 	
\f\r]`,jt=/<(?:(!--|\/[^a-zA-Z])|(\/?[a-zA-Z][^>\s]*)|(\/?$))/g,Qe=/-->/g,ts=/>/g,ht=RegExp(`>|${be}(?:([^\\s"'>=/]+)(${be}*=${be}*(?:[^ 	
\f\r"'\`<>=]|("|')|))|$)`,"g"),es=/'/g,ss=/"/g,Is=/^(?:script|style|textarea|title)$/i,Ds=e=>(t,...s)=>({_$litType$:e,strings:t,values:s}),g=Ds(1),I=Ds(2),Rt=Symbol.for("lit-noChange"),S=Symbol.for("lit-nothing"),is=new WeakMap,_t=bt.createTreeWalker(bt,129);function Ts(e,t){if(!Oe(e)||!e.hasOwnProperty("raw"))throw Error("invalid template strings array");return Je!==void 0?Je.createHTML(t):t}const ci=(e,t)=>{const s=e.length-1,i=[];let a,o=t===2?"<svg>":t===3?"<math>":"",r=jt;for(let n=0;n<s;n++){const c=e[n];let l,h,d=-1,p=0;for(;p<c.length&&(r.lastIndex=p,h=r.exec(c),h!==null);)p=r.lastIndex,r===jt?h[1]==="!--"?r=Qe:h[1]!==void 0?r=ts:h[2]!==void 0?(Is.test(h[2])&&(a=RegExp("</"+h[2],"g")),r=ht):h[3]!==void 0&&(r=ht):r===ht?h[0]===">"?(r=a??jt,d=-1):h[1]===void 0?d=-2:(d=r.lastIndex-h[2].length,l=h[1],r=h[3]===void 0?ht:h[3]==='"'?ss:es):r===ss||r===es?r=ht:r===Qe||r===ts?r=jt:(r=ht,a=void 0);const f=r===ht&&e[n+1].startsWith("/>")?" ":"";o+=r===jt?c+ri:d>=0?(i.push(l),c.slice(0,d)+Ps+c.slice(d)+nt+f):c+nt+(d===-2?n:f)}return[Ts(e,o+(e[s]||"<?>")+(t===2?"</svg>":t===3?"</math>":"")),i]};class Nt{constructor({strings:t,_$litType$:s},i){let a;this.parts=[];let o=0,r=0;const n=t.length-1,c=this.parts,[l,h]=ci(t,s);if(this.el=Nt.createElement(l,i),_t.currentNode=this.el.content,s===2||s===3){const d=this.el.content.firstChild;d.replaceWith(...d.childNodes)}for(;(a=_t.nextNode())!==null&&c.length<n;){if(a.nodeType===1){if(a.hasAttributes())for(const d of a.getAttributeNames())if(d.endsWith(Ps)){const p=h[r++],f=a.getAttribute(d).split(nt),u=/([.?@])?(.*)/.exec(p);c.push({type:1,index:o,name:u[2],strings:f,ctor:u[1]==="."?hi:u[1]==="?"?di:u[1]==="@"?pi:ue}),a.removeAttribute(d)}else d.startsWith(nt)&&(c.push({type:6,index:o}),a.removeAttribute(d));if(Is.test(a.tagName)){const d=a.textContent.split(nt),p=d.length-1;if(p>0){a.textContent=ce?ce.emptyScript:"";for(let f=0;f<p;f++)a.append(d[f],Ut()),_t.nextNode(),c.push({type:2,index:++o});a.append(d[p],Ut())}}}else if(a.nodeType===8)if(a.data===$s)c.push({type:2,index:o});else{let d=-1;for(;(d=a.data.indexOf(nt,d+1))!==-1;)c.push({type:7,index:o}),d+=nt.length-1}o++}}static createElement(t,s){const i=bt.createElement("template");return i.innerHTML=t,i}}function Et(e,t,s=e,i){if(t===Rt)return t;let a=i!==void 0?s._$Co?.[i]:s._$Cl;const o=Wt(t)?void 0:t._$litDirective$;return a?.constructor!==o&&(a?._$AO?.(!1),o===void 0?a=void 0:(a=new o(e),a._$AT(e,s,i)),i!==void 0?(s._$Co??=[])[i]=a:s._$Cl=a),a!==void 0&&(t=Et(e,a._$AS(e,t.values),a,i)),t}class li{constructor(t,s){this._$AV=[],this._$AN=void 0,this._$AD=t,this._$AM=s}get parentNode(){return this._$AM.parentNode}get _$AU(){return this._$AM._$AU}u(t){const{el:{content:s},parts:i}=this._$AD,a=(t?.creationScope??bt).importNode(s,!0);_t.currentNode=a;let o=_t.nextNode(),r=0,n=0,c=i[0];for(;c!==void 0;){if(r===c.index){let l;c.type===2?l=new Vt(o,o.nextSibling,this,t):c.type===1?l=new c.ctor(o,c.name,c.strings,this,t):c.type===6&&(l=new ui(o,this,t)),this._$AV.push(l),c=i[++n]}r!==c?.index&&(o=_t.nextNode(),r++)}return _t.currentNode=bt,a}p(t){let s=0;for(const i of this._$AV)i!==void 0&&(i.strings!==void 0?(i._$AI(t,i,s),s+=i.strings.length-2):i._$AI(t[s])),s++}}class Vt{get _$AU(){return this._$AM?._$AU??this._$Cv}constructor(t,s,i,a){this.type=2,this._$AH=S,this._$AN=void 0,this._$AA=t,this._$AB=s,this._$AM=i,this.options=a,this._$Cv=a?.isConnected??!0}get parentNode(){let t=this._$AA.parentNode;const s=this._$AM;return s!==void 0&&t?.nodeType===11&&(t=s.parentNode),t}get startNode(){return this._$AA}get endNode(){return this._$AB}_$AI(t,s=this){t=Et(this,t,s),Wt(t)?t===S||t==null||t===""?(this._$AH!==S&&this._$AR(),this._$AH=S):t!==this._$AH&&t!==Rt&&this._(t):t._$litType$!==void 0?this.$(t):t.nodeType!==void 0?this.T(t):ni(t)?this.k(t):this._(t)}O(t){return this._$AA.parentNode.insertBefore(t,this._$AB)}T(t){this._$AH!==t&&(this._$AR(),this._$AH=this.O(t))}_(t){this._$AH!==S&&Wt(this._$AH)?this._$AA.nextSibling.data=t:this.T(bt.createTextNode(t)),this._$AH=t}$(t){const{values:s,_$litType$:i}=t,a=typeof i=="number"?this._$AC(t):(i.el===void 0&&(i.el=Nt.createElement(Ts(i.h,i.h[0]),this.options)),i);if(this._$AH?._$AD===a)this._$AH.p(s);else{const o=new li(a,this),r=o.u(this.options);o.p(s),this.T(r),this._$AH=o}}_$AC(t){let s=is.get(t.strings);return s===void 0&&is.set(t.strings,s=new Nt(t)),s}k(t){Oe(this._$AH)||(this._$AH=[],this._$AR());const s=this._$AH;let i,a=0;for(const o of t)a===s.length?s.push(i=new Vt(this.O(Ut()),this.O(Ut()),this,this.options)):i=s[a],i._$AI(o),a++;a<s.length&&(this._$AR(i&&i._$AB.nextSibling,a),s.length=a)}_$AR(t=this._$AA.nextSibling,s){for(this._$AP?.(!1,!0,s);t!==this._$AB;){const i=Ge(t).nextSibling;Ge(t).remove(),t=i}}setConnected(t){this._$AM===void 0&&(this._$Cv=t,this._$AP?.(t))}}class ue{get tagName(){return this.element.tagName}get _$AU(){return this._$AM._$AU}constructor(t,s,i,a,o){this.type=1,this._$AH=S,this._$AN=void 0,this.element=t,this.name=s,this._$AM=a,this.options=o,i.length>2||i[0]!==""||i[1]!==""?(this._$AH=Array(i.length-1).fill(new String),this.strings=i):this._$AH=S}_$AI(t,s=this,i,a){const o=this.strings;let r=!1;if(o===void 0)t=Et(this,t,s,0),r=!Wt(t)||t!==this._$AH&&t!==Rt,r&&(this._$AH=t);else{const n=t;let c,l;for(t=o[0],c=0;c<o.length-1;c++)l=Et(this,n[i+c],s,c),l===Rt&&(l=this._$AH[c]),r||=!Wt(l)||l!==this._$AH[c],l===S?t=S:t!==S&&(t+=(l??"")+o[c+1]),this._$AH[c]=l}r&&!a&&this.j(t)}j(t){t===S?this.element.removeAttribute(this.name):this.element.setAttribute(this.name,t??"")}}class hi extends ue{constructor(){super(...arguments),this.type=3}j(t){this.element[this.name]=t===S?void 0:t}}class di extends ue{constructor(){super(...arguments),this.type=4}j(t){this.element.toggleAttribute(this.name,!!t&&t!==S)}}class pi extends ue{constructor(t,s,i,a,o){super(t,s,i,a,o),this.type=5}_$AI(t,s=this){if((t=Et(this,t,s,0)??S)===Rt)return;const i=this._$AH,a=t===S&&i!==S||t.capture!==i.capture||t.once!==i.once||t.passive!==i.passive,o=t!==S&&(i===S||a);a&&this.element.removeEventListener(this.name,this,i),o&&this.element.addEventListener(this.name,this,t),this._$AH=t}handleEvent(t){typeof this._$AH=="function"?this._$AH.call(this.options?.host??this.element,t):this._$AH.handleEvent(t)}}class ui{constructor(t,s,i){this.element=t,this.type=6,this._$AN=void 0,this._$AM=s,this.options=i}get _$AU(){return this._$AM._$AU}_$AI(t){Et(this,t)}}const fi=je.litHtmlPolyfillSupport;fi?.(Nt,Vt),(je.litHtmlVersions??=[]).push("3.3.2");const _i=(e,t,s)=>{const i=s?.renderBefore??t;let a=i._$litPart$;if(a===void 0){const o=s?.renderBefore??null;i._$litPart$=a=new Vt(t.insertBefore(Ut(),o),o,void 0,s??{})}return a._$AI(e),a};/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const He=globalThis;let F=class extends It{constructor(){super(...arguments),this.renderOptions={host:this},this._$Do=void 0}createRenderRoot(){const t=super.createRenderRoot();return this.renderOptions.renderBefore??=t.firstChild,t}update(t){const s=this.render();this.hasUpdated||(this.renderOptions.isConnected=this.isConnected),super.update(t),this._$Do=_i(s,this.renderRoot,this.renderOptions)}connectedCallback(){super.connectedCallback(),this._$Do?.setConnected(!0)}disconnectedCallback(){super.disconnectedCallback(),this._$Do?.setConnected(!1)}render(){return Rt}};F._$litElement$=!0,F.finalized=!0,He.litElementHydrateSupport?.({LitElement:F});const mi=He.litElementPolyfillSupport;mi?.({LitElement:F});(He.litElementVersions??=[]).push("4.2.2");/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const wt=e=>(t,s)=>{s!==void 0?s.addInitializer(()=>{customElements.define(e,t)}):customElements.define(e,t)};/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const gi={attribute:!0,type:String,converter:ne,reflect:!1,hasChanged:Ae},vi=(e=gi,t,s)=>{const{kind:i,metadata:a}=s;let o=globalThis.litPropertyMetadata.get(a);if(o===void 0&&globalThis.litPropertyMetadata.set(a,o=new Map),i==="setter"&&((e=Object.create(e)).wrapped=!0),o.set(s.name,e),i==="accessor"){const{name:r}=s;return{set(n){const c=t.get.call(this);t.set.call(this,n),this.requestUpdate(r,c,e,!0,n)},init(n){return n!==void 0&&this.C(r,void 0,e,n),n}}}if(i==="setter"){const{name:r}=s;return function(n){const c=this[r];t.call(this,n),this.requestUpdate(r,c,e,!0,n)}}throw Error("Unsupported decorator location: "+i)};function Be(e){return(t,s)=>typeof s=="object"?vi(e,t,s):((i,a,o)=>{const r=a.hasOwnProperty(o);return a.constructor.createProperty(o,i),r?Object.getOwnPropertyDescriptor(a,o):void 0})(e,t,s)}/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */function M(e){return Be({...e,state:!0,attribute:!1})}/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */const bi=(e,t,s)=>(s.configurable=!0,s.enumerable=!0,Reflect.decorate&&typeof t!="object"&&Object.defineProperty(e,t,s),s);/**
 * @license
 * Copyright 2017 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */function fe(e,t){return(s,i,a)=>{const o=r=>r.renderRoot?.querySelector(e)??null;return bi(s,i,{get(){return o(this)}})}}/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */let Rs=class extends Event{constructor(t,s,i,a){super("context-request",{bubbles:!0,composed:!0}),this.context=t,this.contextTarget=s,this.callback=i,this.subscribe=a??!1}};/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 *//**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */let gt=class{constructor(t,s,i,a){if(this.subscribe=!1,this.provided=!1,this.value=void 0,this.t=(o,r)=>{this.unsubscribe&&(this.unsubscribe!==r&&(this.provided=!1,this.unsubscribe()),this.subscribe||this.unsubscribe()),this.value=o,this.host.requestUpdate(),this.provided&&!this.subscribe||(this.provided=!0,this.callback&&this.callback(o,r)),this.unsubscribe=r},this.host=t,s.context!==void 0){const o=s;this.context=o.context,this.callback=o.callback,this.subscribe=o.subscribe??!1}else this.context=s,this.callback=i,this.subscribe=a??!1;this.host.addController(this)}hostConnected(){this.dispatchRequest()}hostDisconnected(){this.unsubscribe&&(this.unsubscribe(),this.unsubscribe=void 0)}dispatchRequest(){this.host.dispatchEvent(new Rs(this.context,this.host,this.t,this.subscribe))}};/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */class yi{get value(){return this.o}set value(t){this.setValue(t)}setValue(t,s=!1){const i=s||!Object.is(t,this.o);this.o=t,i&&this.updateObservers()}constructor(t){this.subscriptions=new Map,this.updateObservers=()=>{for(const[s,{disposer:i}]of this.subscriptions)s(this.o,i)},t!==void 0&&(this.value=t)}addCallback(t,s,i){if(!i)return void t(this.value);this.subscriptions.has(t)||this.subscriptions.set(t,{disposer:()=>{this.subscriptions.delete(t)},consumerHost:s});const{disposer:a}=this.subscriptions.get(t);t(this.value,a)}clearCallbacks(){this.subscriptions.clear()}}/**
 * @license
 * Copyright 2021 Google LLC
 * SPDX-License-Identifier: BSD-3-Clause
 */class wi extends Event{constructor(t,s){super("context-provider",{bubbles:!0,composed:!0}),this.context=t,this.contextTarget=s}}class ye extends yi{constructor(t,s,i){super(s.context!==void 0?s.initialValue:i),this.onContextRequest=a=>{if(a.context!==this.context)return;const o=a.contextTarget??a.composedPath()[0];o!==this.host&&(a.stopPropagation(),this.addCallback(a.callback,o,a.subscribe))},this.onProviderRequest=a=>{if(a.context!==this.context||(a.contextTarget??a.composedPath()[0])===this.host)return;const o=new Set;for(const[r,{consumerHost:n}]of this.subscriptions)o.has(r)||(o.add(r),n.dispatchEvent(new Rs(this.context,n,r,!0)));a.stopPropagation()},this.host=t,s.context!==void 0?this.context=s.context:this.context=s,this.attachListeners(),this.host.addController?.(this)}attachListeners(){this.host.addEventListener("context-request",this.onContextRequest),this.host.addEventListener("context-provider",this.onProviderRequest)}hostConnected(){this.host.dispatchEvent(new wi(this.context,this.host))}}const Lt="drawing-context",xi={normal:"Normal",multiply:"Multiply",screen:"Screen",overlay:"Overlay",darken:"Darken",lighten:"Lighten","soft-light":"Soft Light"},Ci={normal:"source-over",multiply:"multiply",screen:"screen",overlay:"overlay",darken:"darken",lighten:"lighten","soft-light":"soft-light"};function Yt(e){return Ci[e]}const Mi={linear:e=>e,light:e=>Math.pow(e,.5),heavy:e=>Math.pow(e,2)};function ki(e){return Math.max(2,Math.round(e/2)*2)}const Si={round:{aspect:!1,rotation:!1,bristles:!1},flat:{aspect:!0,rotation:!0,bristles:!1},chisel:{aspect:!0,rotation:!0,bristles:!1},calligraphy:{aspect:!0,rotation:!0,bristles:!1},fan:{aspect:!1,rotation:!0,bristles:!0},splatter:{aspect:!1,rotation:!0,bristles:!0}},Pi={round:{shape:"round",aspect:1,angle:0,orientation:"fixed"},flat:{shape:"flat",aspect:3,angle:0,orientation:"direction"},chisel:{shape:"chisel",aspect:2.5,angle:0,orientation:"direction"},calligraphy:{shape:"calligraphy",aspect:4,angle:45,orientation:"fixed"},fan:{shape:"fan",aspect:1,angle:0,orientation:"direction",bristles:8,spread:120},splatter:{shape:"splatter",aspect:1,angle:0,orientation:"fixed",bristles:12,spread:.8}};function $i(e){return Si[e]}function X(e){return{...Pi[e]}}const we={depletion:0,depletionLength:500,buildup:0,wetness:0},et=[{id:"round",name:"Round",category:"basic",descriptor:{size:4,opacity:1,flow:1,hardness:1,spacing:.15,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("round"),ink:{...we}}},{id:"soft-round",name:"Soft Round",category:"basic",descriptor:{size:20,opacity:1,flow:.6,hardness:.3,spacing:.12,pressureSize:!0,pressureOpacity:!0,pressureCurve:"light",tip:X("round"),ink:{...we}}},{id:"flat",name:"Flat",category:"artistic",descriptor:{size:30,opacity:1,flow:.8,hardness:.9,spacing:.1,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("flat"),ink:{depletion:.3,depletionLength:800,buildup:0,wetness:0}}},{id:"chisel",name:"Chisel",category:"artistic",descriptor:{size:24,opacity:1,flow:.9,hardness:.95,spacing:.1,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("chisel"),ink:{depletion:.2,depletionLength:600,buildup:.3,wetness:0}}},{id:"calligraphy",name:"Calligraphy",category:"artistic",descriptor:{size:20,opacity:1,flow:1,hardness:1,spacing:.08,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("calligraphy"),ink:{...we}}},{id:"fan",name:"Fan",category:"artistic",descriptor:{size:40,opacity:1,flow:.7,hardness:.8,spacing:.15,pressureSize:!0,pressureOpacity:!1,pressureCurve:"linear",tip:X("fan"),ink:{depletion:.5,depletionLength:600,buildup:0,wetness:0}}},{id:"splatter",name:"Splatter",category:"effects",descriptor:{size:50,opacity:.8,flow:.6,hardness:.7,spacing:.25,pressureSize:!1,pressureOpacity:!0,pressureCurve:"linear",tip:X("splatter"),ink:{depletion:.7,depletionLength:400,buildup:0,wetness:0}}},{id:"dry-brush",name:"Dry Brush",category:"artistic",descriptor:{size:25,opacity:1,flow:.5,hardness:.8,spacing:.12,pressureSize:!0,pressureOpacity:!0,pressureCurve:"heavy",tip:X("round"),ink:{depletion:.8,depletionLength:300,buildup:.4,wetness:0}}},{id:"wet-brush",name:"Wet Brush",category:"artistic",descriptor:{size:20,opacity:.8,flow:.7,hardness:.5,spacing:.1,pressureSize:!1,pressureOpacity:!1,pressureCurve:"linear",tip:X("round"),ink:{depletion:0,depletionLength:500,buildup:.2,wetness:.91}}}];function Ii(e){return et.find(t=>t.id===e)}function ie(){return{...et[0].descriptor,tip:{...et[0].descriptor.tip},ink:{...et[0].descriptor.ink}}}class N extends Error{constructor(t,s){super(t),this.cause=s,this.name="StorageError"}}class qt extends N{constructor(){super(...arguments),this.name="StorageNotFoundError"}}class Es extends N{constructor(){super(...arguments),this.name="StorageQuotaError"}}class Di extends N{constructor(){super(...arguments),this.name="StorageConflictError"}}const Ti=20;class Ri{constructor(t,s){this._storage=t,this._maxStamps=s?.maxStampsPerProject??Ti}get storage(){return this._storage}async deleteProject(t){const s=new Set,[i,a,o,r]=await Promise.all([this._storage.state.get(t).catch(()=>null),this._storage.history.getEntries(t).catch(()=>[]),this._storage.stamps.list(t).catch(()=>[]),this._storage.projects.get(t).catch(()=>null)]);r?.thumbnailRef&&s.add(r.thumbnailRef),i?.layers.forEach(n=>s.add(n.imageBlobRef));for(const n of a)Dt(n.entry,s);o.forEach(n=>s.add(n.blobRef)),await this._storage.projects.delete(t),await Promise.all([this._storage.state.delete(t),this._storage.history.deleteForProject(t),this._storage.stamps.deleteForProject(t)]).catch(n=>console.error("Cascade delete partial failure:",n)),s.size>0&&this._storage.blobs.deleteMany([...s]).catch(()=>{})}async addStamp(t,s){const i=await this._storage.stamps.add(t,s),a=await this._storage.stamps.list(t);if(a.length>this._maxStamps){const o=a.filter(r=>r.id!==i.id).sort((r,n)=>r.createdAt-n.createdAt).slice(0,a.length-this._maxStamps);for(const r of o)await this._storage.stamps.delete(r.id)}return i}async collectGarbage(){if(!this._storage.blobs.gc)return 0;const t=await this._storage.projects.list(),s=new Set;for(const i of t){const[a,o,r]=await Promise.all([this._storage.state.get(i.id),this._storage.history.getEntries(i.id),this._storage.stamps.list(i.id)]);i.thumbnailRef&&s.add(i.thumbnailRef),a?.layers.forEach(n=>s.add(n.imageBlobRef)),o.forEach(n=>Dt(n.entry,s)),r.forEach(n=>s.add(n.blobRef)),await new Promise(n=>setTimeout(n,0))}return this._storage.blobs.gc(s)}}function Dt(e,t){switch(e.type){case"draw":case"patch":case"transform":t.add(e.before.blobRef),t.add(e.after.blobRef);break;case"add-layer":case"delete-layer":t.add(e.layer.imageData.blobRef);break;case"crop":case"merge":for(const s of e.beforeLayers)t.add(s.imageData.blobRef);for(const s of e.afterLayers)t.add(s.imageData.blobRef);break}}const Ls="storage-backend",zs="project-service";class Ei{constructor(t){this._newId=t,this._blobs=new Map}async put(t){const s=this._newId();return this._blobs.set(s,t instanceof Blob?t:new Blob([t])),s}async get(t){const s=this._blobs.get(t);if(!s)throw new qt(`Blob ${t} not found`);return s}async delete(t){this._blobs.delete(t)}async deleteMany(t){for(const s of t)this._blobs.delete(s)}async gc(t){let s=0;for(const i of[...this._blobs.keys()])t.has(i)||(this._blobs.delete(i),s++);return s}}class Li{constructor(t){this._newId=t,this._projects=new Map}async list(t){const s=Array.from(this._projects.values()),i=t?.orderBy??"updatedAt",a=t?.direction==="asc"?1:-1;return s.sort((o,r)=>a*(o[i]-r[i])),s}async get(t){return this._projects.get(t)??null}async create(t){const s=Date.now(),i={id:this._newId(),name:t.name,createdAt:s,updatedAt:s,thumbnailRef:t.thumbnailRef??null};return this._projects.set(i.id,i),i}async update(t,s){const i=this._projects.get(t);if(!i)throw new qt(`Project ${t}`);return s.name!==void 0&&(i.name=s.name),s.thumbnailRef!==void 0&&(i.thumbnailRef=s.thumbnailRef),i.updatedAt=Date.now(),i}async delete(t){this._projects.delete(t)}}class zi{constructor(){this._states=new Map}async get(t){return this._states.get(t)??null}async save(t){this._states.set(t.projectId,t)}async delete(t){this._states.delete(t)}}class Ai{constructor(){this._entries=new Map}async getEntries(t){return[...this._entries.get(t)??[]].sort((s,i)=>s.index-i.index)}async putEntries(t,s){const i=this._entries.get(t)??[];this._entries.set(t,[...i,...s])}async replaceAll(t,s){this._entries.set(t,[...s])}async updateEntries(t,s,i){const a=new Set(s),o=(this._entries.get(t)??[]).filter(r=>!a.has(r.index));this._entries.set(t,[...o,...i])}async deleteForProject(t){this._entries.delete(t)}}class ji{constructor(t,s){this._blobs=t,this._newId=s,this._stamps=new Map}async list(t){return Array.from(this._stamps.values()).filter(s=>s.projectId===t).sort((s,i)=>i.createdAt-s.createdAt)}async add(t,s){const i=await this._blobs.put(s),a={id:this._newId(),projectId:t,blobRef:i,createdAt:Date.now()};return this._stamps.set(a.id,a),a}async delete(t){this._stamps.delete(t)}async deleteForProject(t){const s=[];for(const[i,a]of this._stamps)a.projectId===t&&(s.push(a.blobRef),this._stamps.delete(i));s.length>0&&this._blobs.deleteMany(s).catch(()=>{})}}class Oi{constructor(t=()=>crypto.randomUUID()){this.blobs=new Ei(t),this.projects=new Li(t),this.state=new zi,this.history=new Ai,this.stamps=new ji(this.blobs,t)}async init(){}async dispose(){}}function $(e){if(e instanceof DOMException)switch(e.name){case"QuotaExceededError":return new Es(e.message,e);case"NotFoundError":return new qt(e.message,e);case"ConstraintError":return new Di(e.message,e);default:return new N(e.message,e)}return e instanceof Error?new N(e.message,e):new N(String(e))}function mt(){if(typeof crypto<"u"&&typeof crypto.randomUUID=="function")return crypto.randomUUID();const e=new Uint8Array(16);crypto.getRandomValues(e),e[6]=e[6]&15|64,e[8]=e[8]&63|128;const t=Array.from(e,s=>s.toString(16).padStart(2,"0")).join("");return`${t.slice(0,8)}-${t.slice(8,12)}-${t.slice(12,16)}-${t.slice(16,20)}-${t.slice(20)}`}function Hi(e,t,s){const i=e.createObjectStore("blobs");if(s<1)return;function a(u){if(u?.blob){const _=mt();return i.put(u.blob,_),u.blobRef=_,delete u.blob,!0}return!1}function o(u){return a(u?.imageData)}const n=t.objectStore("projects").openCursor();n.onsuccess=function(){const u=n.result;if(!u)return;const _=u.value;if(_.thumbnail){const m=mt();i.put(_.thumbnail,m),_.thumbnailRef=m,delete _.thumbnail,u.update(_)}u.continue()};const l=t.objectStore("project-state").openCursor();l.onsuccess=function(){const u=l.result;if(!u)return;const _=u.value;let m=!1;for(const v of _.layers??[])if(v.imageBlob){const b=mt();i.put(v.imageBlob,b),v.imageBlobRef=b,delete v.imageBlob,m=!0}m&&u.update(_),u.continue()};const d=t.objectStore("project-history").openCursor();d.onsuccess=function(){const u=d.result;if(!u)return;const _=u.value,m=_.entry;let v=!1;switch(m?.type){case"draw":case"transform":case"patch":a(m.before)&&(v=!0),a(m.after)&&(v=!0);break;case"add-layer":case"delete-layer":o(m.layer)&&(v=!0);break;case"crop":for(const b of m.beforeLayers??[])o(b)&&(v=!0);for(const b of m.afterLayers??[])o(b)&&(v=!0);break}v&&u.update(_),u.continue()};const f=t.objectStore("project-stamps").openCursor();f.onsuccess=function(){const u=f.result;if(!u)return;const _=u.value;if(_.blob){const m=mt();i.put(_.blob,m),_.blobRef=m,delete _.blob,u.update(_)}u.continue()}}const Mt="blobs";class Bi{constructor(t){this._db=t}async put(t){const s=mt();return await this._tx("readwrite",i=>i.put(t,s)),s}async get(t){const s=await this._tx("readonly",i=>i.get(t));if(s===void 0)throw new qt(`Blob not found: ${t}`);return s instanceof Blob?s:new Blob([s])}async delete(t){await this._tx("readwrite",s=>s.delete(t))}async deleteMany(t){t.length!==0&&await new Promise((s,i)=>{const a=this._db.transaction(Mt,"readwrite"),o=a.objectStore(Mt);for(const r of t)o.delete(r);a.oncomplete=()=>s(),a.onerror=()=>i($(a.error))})}async gc(t){let s=0;const i=[];return await new Promise((a,o)=>{const r=this._db.transaction(Mt,"readonly"),n=r.objectStore(Mt).openKeyCursor();n.onsuccess=()=>{const c=n.result;if(c){const l=c.key;t.has(l)||i.push(l),c.continue()}},r.oncomplete=()=>a(),r.onerror=()=>o($(r.error))}),i.length>0&&(await this.deleteMany(i),s=i.length),s}_tx(t,s){return new Promise((i,a)=>{const o=this._db.transaction(Mt,t),r=s(o.objectStore(Mt));r.onsuccess=()=>i(r.result),o.onerror=()=>a($(o.error))})}}const q="projects";class Yi{constructor(t){this._db=t}async list(t){const s=t?.orderBy??"updatedAt",i=t?.direction??"desc";return new Promise((a,o)=>{const n=this._db.transaction(q,"readonly").objectStore(q),c=[];if(s==="updatedAt"){const h=n.index("updatedAt").openCursor(null,i==="desc"?"prev":"next");h.onsuccess=()=>{const d=h.result;d?(c.push(d.value),d.continue()):a(c)},h.onerror=()=>o($(h.error))}else{const l=n.getAll();l.onsuccess=()=>{const h=l.result;h.sort((d,p)=>i==="desc"?p.createdAt-d.createdAt:d.createdAt-p.createdAt),a(h)},l.onerror=()=>o($(l.error))}})}async get(t){return new Promise((s,i)=>{const o=this._db.transaction(q,"readonly").objectStore(q).get(t);o.onsuccess=()=>s(o.result??null),o.onerror=()=>i($(o.error))})}async create(t){const s=Date.now(),i={id:mt(),name:t.name,createdAt:s,updatedAt:s,thumbnailRef:t.thumbnailRef??null};return await new Promise((a,o)=>{const r=this._db.transaction(q,"readwrite");r.objectStore(q).add(i),r.oncomplete=()=>a(),r.onerror=()=>o($(r.error))}),i}async update(t,s){return new Promise((i,a)=>{const o=this._db.transaction(q,"readwrite"),r=o.objectStore(q);let n;const c=r.get(t);c.onsuccess=()=>{const l=c.result;if(!l){a(new qt(`Project ${t} not found`));return}s.name!==void 0&&(l.name=s.name),s.thumbnailRef!==void 0&&(l.thumbnailRef=s.thumbnailRef),l.updatedAt=Date.now(),n=l,r.put(l)},c.onerror=()=>a($(c.error)),o.oncomplete=()=>i(n),o.onerror=()=>a($(o.error))})}async delete(t){await new Promise((s,i)=>{const a=this._db.transaction(q,"readwrite");a.objectStore(q).delete(t),a.oncomplete=()=>s(),a.onerror=()=>i($(a.error))})}}const kt="project-state";class Xi{constructor(t){this._db=t}async get(t){return new Promise((s,i)=>{const o=this._db.transaction(kt,"readonly").objectStore(kt).get(t);o.onsuccess=()=>s(o.result??null),o.onerror=()=>i($(o.error))})}async save(t){await new Promise((s,i)=>{const a=this._db.transaction(kt,"readwrite");a.objectStore(kt).put(t),a.oncomplete=()=>s(),a.onerror=()=>i($(a.error))})}async delete(t){await new Promise((s,i)=>{const a=this._db.transaction(kt,"readwrite");a.objectStore(kt).delete(t),a.oncomplete=()=>s(),a.onerror=()=>i($(a.error))})}}const Z="project-history";class Ui{constructor(t){this._db=t}async getEntries(t){return new Promise((s,i)=>{const o=this._db.transaction(Z,"readonly").objectStore(Z).index("projectId"),r=[],n=o.openCursor(IDBKeyRange.only(t));n.onsuccess=()=>{const c=n.result;c?(r.push(c.value),c.continue()):(r.sort((l,h)=>l.index-h.index),s(r))},n.onerror=()=>i($(n.error))})}async putEntries(t,s){s.length!==0&&await new Promise((i,a)=>{const o=this._db.transaction(Z,"readwrite"),r=o.objectStore(Z);for(const n of s){const{id:c,...l}=n;r.add({...l,projectId:t})}o.oncomplete=()=>i(),o.onerror=()=>a($(o.error))})}async replaceAll(t,s){await new Promise((i,a)=>{const o=this._db.transaction(Z,"readwrite"),r=o.objectStore(Z),c=r.index("projectId").openCursor(IDBKeyRange.only(t));c.onsuccess=()=>{const l=c.result;if(l)l.delete(),l.continue();else for(const h of s){const{id:d,...p}=h;r.add({...p,projectId:t})}},c.onerror=()=>a($(c.error)),o.oncomplete=()=>i(),o.onerror=()=>a($(o.error))})}async updateEntries(t,s,i){s.length===0&&i.length===0||await new Promise((a,o)=>{const r=this._db.transaction(Z,"readwrite"),n=r.objectStore(Z);if(s.length>0){const c=new Set(s),l=n.index("projectId").openCursor(IDBKeyRange.only(t));l.onsuccess=()=>{const h=l.result;h&&(c.has(h.value.index)&&h.delete(),h.continue())},l.onerror=()=>o($(l.error))}for(const c of i){const{id:l,...h}=c;n.add({...h,projectId:t})}r.oncomplete=()=>a(),r.onerror=()=>o($(r.error)),r.onabort=()=>o($(r.error))})}async deleteForProject(t){await new Promise((s,i)=>{const a=this._db.transaction(Z,"readwrite"),r=a.objectStore(Z).index("projectId").openCursor(IDBKeyRange.only(t));r.onsuccess=()=>{const n=r.result;n&&(n.delete(),n.continue())},r.onerror=()=>i($(r.error)),a.oncomplete=()=>s(),a.onerror=()=>i($(a.error))})}}const K="project-stamps";class Wi{constructor(t,s){this._db=t,this._blobs=s}async list(t){return new Promise((s,i)=>{const o=this._db.transaction(K,"readonly").objectStore(K).index("projectId"),r=[],n=o.openCursor(IDBKeyRange.only(t));n.onsuccess=()=>{const c=n.result;c?(r.push(c.value),c.continue()):(r.sort((l,h)=>h.createdAt-l.createdAt),s(r))},n.onerror=()=>i($(n.error))})}async add(t,s){const i=await this._blobs.put(s),a={id:mt(),projectId:t,blobRef:i,createdAt:Date.now()};return await new Promise((o,r)=>{const n=this._db.transaction(K,"readwrite");n.objectStore(K).add(a),n.oncomplete=()=>o(),n.onerror=()=>r($(n.error))}),a}async delete(t){const s=await new Promise((i,a)=>{const r=this._db.transaction(K,"readonly").objectStore(K).get(t);r.onsuccess=()=>{const n=r.result;i(n?.blobRef??null)},r.onerror=()=>a($(r.error))});await new Promise((i,a)=>{const o=this._db.transaction(K,"readwrite");o.objectStore(K).delete(t),o.oncomplete=()=>i(),o.onerror=()=>a($(o.error))}),s&&this._blobs.delete(s).catch(()=>{})}async deleteForProject(t){const s=[];await new Promise((i,a)=>{const o=this._db.transaction(K,"readwrite"),n=o.objectStore(K).index("projectId").openCursor(IDBKeyRange.only(t));n.onsuccess=()=>{const c=n.result;if(c){const l=c.value;s.push(l.blobRef),c.delete(),c.continue()}},n.onerror=()=>a($(n.error)),o.oncomplete=()=>i(),o.onerror=()=>a($(o.error))}),s.length>0&&this._blobs.deleteMany(s).catch(()=>{})}}const Ni="ketchup-projects",Fi=4;class Vi{constructor(t){this._db=null,this._dbName=t?.dbName??Ni,this._version=t?.version??Fi}get projects(){if(!this._projects)throw new N("Backend not initialized — call init() first");return this._projects}get state(){if(!this._state)throw new N("Backend not initialized — call init() first");return this._state}get history(){if(!this._history)throw new N("Backend not initialized — call init() first");return this._history}get stamps(){if(!this._stamps)throw new N("Backend not initialized — call init() first");return this._stamps}get blobs(){if(!this._blobs)throw new N("Backend not initialized — call init() first");return this._blobs}async init(){const t=indexedDB.deleteDatabase("ketchup-stamps");t.onerror=()=>{},t.onblocked=()=>{},this._db=await new Promise((a,o)=>{const r=indexedDB.open(this._dbName,this._version);r.onupgradeneeded=n=>{const c=r.result,l=r.transaction,h=n.oldVersion;c.objectStoreNames.contains("projects")||c.createObjectStore("projects",{keyPath:"id"}).createIndex("updatedAt","updatedAt"),c.objectStoreNames.contains("project-state")||c.createObjectStore("project-state",{keyPath:"projectId"}),c.objectStoreNames.contains("project-history")||c.createObjectStore("project-history",{keyPath:"id",autoIncrement:!0}).createIndex("projectId","projectId"),c.objectStoreNames.contains("project-stamps")||c.createObjectStore("project-stamps",{keyPath:"id"}).createIndex("projectId","projectId"),h<4&&!c.objectStoreNames.contains("blobs")&&Hi(c,l,h)},r.onsuccess=()=>a(r.result),r.onerror=()=>o(r.error)});const s=this._db,i=new Bi(s);this._blobs=i,this._projects=new Yi(s),this._state=new Xi(s),this._history=new Ui(s),this._stamps=new Wi(s,i)}async dispose(){this._db&&(this._db.close(),this._db=null)}}async function Ye(e){return new Promise((t,s)=>{e.toBlob(i=>{i?t(i):s(new Error("canvas.toBlob returned null"))},"image/png")})}async function qi(e,t,s){const i=await createImageBitmap(e),a=document.createElement("canvas");return a.width=t,a.height=s,a.getContext("2d").drawImage(i,0,0),i.close(),a}async function As(e){const t=document.createElement("canvas");return t.width=e.width,t.height=e.height,t.getContext("2d").putImageData(e,0,0),Ye(t)}async function Zi(e,t,s){const i=await createImageBitmap(e),a=document.createElement("canvas");a.width=t,a.height=s;const o=a.getContext("2d");return o.drawImage(i,0,0),i.close(),o.getImageData(0,0,t,s)}function Pe(e){return new Uint32Array(e.data.buffer,e.data.byteOffset,e.width*e.height)}function $e(e,t){const s=e.width,i=e.height,a=Pe(e),o=Pe(t);let r=s,n=-1,c=-1,l=-1;for(let h=0;h<i;h++){const d=h*s;let p=0;for(;p<s&&a[d+p]===o[d+p];)p++;if(p===s)continue;c<0&&(c=h),l=h,p<r&&(r=p);let f=s-1;for(;f>n&&a[d+f]===o[d+f];)f--;f>n&&(n=f)}return c<0?null:{x:r,y:c,w:n-r+1,h:l-c+1}}function Gt(e,t){if(t.x===0&&t.y===0&&t.w===e.width&&t.h===e.height)return e;const s=new ImageData(t.w,t.h),i=t.w*4;for(let a=0;a<t.h;a++){const o=((t.y+a)*e.width+t.x)*4;s.data.set(e.data.subarray(o,o+i),a*i)}return s}function as(e){const t=Pe(e);let s=2166136261,i=2654435769;for(let a=0;a<t.length;a++){const o=t[a];s=Math.imul(s^o,16777619),s^=s>>>13,i=Math.imul(i^o,1540483477),i^=i>>>15}return`${e.width}x${e.height}:${(s>>>0).toString(16)}:${(i>>>0).toString(16)}`}async function pt(e,t){const s=await As(e),i=await t.put(s);return{width:e.width,height:e.height,blobRef:i}}async function ut(e,t){const s=await t.get(e.blobRef);return Zi(s,e.width,e.height)}async function St(e,t){return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode,imageData:await pt(e.imageData,t)}}async function Pt(e,t){return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode??"normal",imageData:await ut(e.imageData,t)}}async function Ki(e,t,s){const i=await As(t),a=await s.put(i);return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode??"normal",imageBlobRef:a}}async function Gi(e,t,s,i){const a=await i.get(e.imageBlobRef),o=await qi(a,t,s);return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode??"normal",canvas:o}}async function Ji(e,t){switch(e.type){case"draw":{const[s,i]=await Promise.all([pt(e.before,t),pt(e.after,t)]);return{type:"draw",layerId:e.layerId,before:s,after:i}}case"patch":{const[s,i]=await Promise.all([pt(e.before,t),pt(e.after,t)]);return{type:"patch",layerId:e.layerId,x:e.x,y:e.y,before:s,after:i}}case"add-layer":return{type:"add-layer",layer:await St(e.layer,t),index:e.index};case"delete-layer":return{type:"delete-layer",layer:await St(e.layer,t),index:e.index};case"crop":{const[s,i]=await Promise.all([Promise.all(e.beforeLayers.map(a=>St(a,t))),Promise.all(e.afterLayers.map(a=>St(a,t)))]);return{type:"crop",beforeLayers:s,afterLayers:i,beforeWidth:e.beforeWidth,beforeHeight:e.beforeHeight,afterWidth:e.afterWidth,afterHeight:e.afterHeight}}case"merge":{const[s,i]=await Promise.all([Promise.all(e.beforeLayers.map(a=>St(a,t))),Promise.all(e.afterLayers.map(a=>St(a,t)))]);return{type:"merge",beforeLayers:s,afterLayers:i,previousActiveLayerId:e.previousActiveLayerId,afterActiveLayerId:e.afterActiveLayerId}}case"reorder":case"visibility":case"opacity":case"rename":case"blend-mode":return e;case"transform":{const[s,i]=await Promise.all([pt(e.before,t),pt(e.after,t)]);return{type:"transform",layerId:e.layerId,before:s,after:i}}}}async function Qi(e,t){switch(e.type){case"draw":{const[s,i]=await Promise.all([ut(e.before,t),ut(e.after,t)]);return{type:"draw",layerId:e.layerId,before:s,after:i}}case"patch":{const[s,i]=await Promise.all([ut(e.before,t),ut(e.after,t)]);return{type:"patch",layerId:e.layerId,x:e.x,y:e.y,before:s,after:i}}case"add-layer":return{type:"add-layer",layer:await Pt(e.layer,t),index:e.index};case"delete-layer":return{type:"delete-layer",layer:await Pt(e.layer,t),index:e.index};case"crop":{const[s,i]=await Promise.all([Promise.all(e.beforeLayers.map(a=>Pt(a,t))),Promise.all(e.afterLayers.map(a=>Pt(a,t)))]);return{type:"crop",beforeLayers:s,afterLayers:i,beforeWidth:e.beforeWidth,beforeHeight:e.beforeHeight,afterWidth:e.afterWidth,afterHeight:e.afterHeight}}case"merge":{const[s,i]=await Promise.all([Promise.all(e.beforeLayers.map(a=>Pt(a,t))),Promise.all(e.afterLayers.map(a=>Pt(a,t)))]);return{type:"merge",beforeLayers:s,afterLayers:i,previousActiveLayerId:e.previousActiveLayerId,afterActiveLayerId:e.afterActiveLayerId}}case"reorder":case"visibility":case"opacity":case"rename":return e;case"blend-mode":return{type:"blend-mode",layerId:e.layerId,before:e.before,after:e.after};case"transform":{const[s,i]=await Promise.all([ut(e.before,t),ut(e.after,t)]);return{type:"transform",layerId:e.layerId,before:s,after:i}}}}const Ot={select:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square" stroke-linejoin="miter">
      <rect x="4" y="4" width="16" height="16" stroke-dasharray="4 4" />
    </svg>`,move:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="5 9 2 12 5 15"/>
      <polyline points="9 5 12 2 15 5"/>
      <polyline points="15 19 12 22 9 19"/>
      <polyline points="19 9 22 12 19 15"/>
      <line x1="2" y1="12" x2="22" y2="12"/>
      <line x1="12" y1="2" x2="12" y2="22"/>
    </svg>`,pencil:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M17 3a2.83 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>
    </svg>`,eraser:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m7 21-4.3-4.3c-1-1-1-2.5 0-3.4l9.6-9.6c1-1 2.5-1 3.4 0l5.6 5.6c1 1 1 2.5 0 3.4L13 21"/>
      <path d="M22 21H7"/>
      <path d="m5 11 9 9"/>
    </svg>`,line:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <line x1="5" y1="19" x2="19" y2="5"/>
    </svg>`,rectangle:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2"/>
    </svg>`,circle:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
      <circle cx="12" cy="12" r="10"/>
    </svg>`,triangle:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 3L22 21H2z"/>
    </svg>`,diamond:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2L22 12 12 22 2 12z"/>
    </svg>`,pentagon:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2L22 9.3 18.2 21H5.8L2 9.3z"/>
    </svg>`,hexagon:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M7 2H17L22 12 17 22H7L2 12z"/>
    </svg>`,star:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 2.5l2.9 5.9 6.5.9-4.7 4.6 1.1 6.5-5.8-3-5.8 3 1.1-6.5-4.7-4.6 6.5-.9z"/>
    </svg>`,heart:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8l1.1 1.1L12 21.2l7.8-7.7 1.1-1.1a5.5 5.5 0 0 0-.1-7.8z"/>
    </svg>`,arrow:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 9h10V4l8 8-8 8v-5H3z"/>
    </svg>`,fill:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m19 11-8-8-8.6 8.6a2 2 0 0 0 0 2.8l5.2 5.2c.8.8 2 .8 2.8 0L19 11Z"/>
      <path d="m5 2 5 5"/>
      <path d="M2 13h15"/>
      <path d="M22 20a2 2 0 1 1-4 0c0-1.6 1.7-2.4 2-4 .3 1.6 2 2.4 2 4Z"/>
    </svg>`,stamp:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M2 22h20"/>
      <path d="M6 12v-4c0-2.2 1.8-4 4-4h4c2.2 0 4 1.8 4 4v4"/>
      <path d="M6 12h12a2 2 0 0 1 2 2v2a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2v-2a2 2 0 0 1 2-2Z"/>
      <path d="M9 13v-1"/>
      <path d="M15 13v-1"/>
    </svg>`,eyedropper:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="m2 22 1-1h3l9-9"/>
      <path d="M3 21v-3l9-9"/>
      <path d="m15 6 3.4-3.4a2.1 2.1 0 1 1 3 3L18 9"/>
      <path d="m15 6 3 3"/>
    </svg>`,text:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="4 7 4 4 20 4 20 7"/>
      <line x1="12" y1="4" x2="12" y2="21"/>
      <line x1="8" y1="21" x2="16" y2="21"/>
    </svg>`,hand:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M18 11V6a2 2 0 0 0-4 0v1"/>
      <path d="M14 10V4a2 2 0 0 0-4 0v6"/>
      <path d="M10 10.5V6a2 2 0 0 0-4 0v8"/>
      <path d="M18 8a2 2 0 0 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15"/>
    </svg>`,crop:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M6 2v4H2"/>
      <path d="M6 6h12v12"/>
      <path d="M18 22v-4h4"/>
      <path d="M2 6h4v12h12"/>
    </svg>`},os=I`
  <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M12 2.5 17 10H7z"/>
    <circle cx="6.5" cy="17" r="4"/>
    <rect x="13" y="13" width="8" height="8" rx="1"/>
  </svg>`,Y={undo:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="1 4 1 10 7 10"/>
      <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/>
    </svg>`,redo:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="23 4 23 10 17 10"/>
      <path d="M20.49 15a9 9 0 1 1-2.13-9.36L23 10"/>
    </svg>`,save:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
      <polyline points="7 10 12 15 17 10"/>
      <line x1="12" y1="15" x2="12" y2="3"/>
    </svg>`,clear:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3 6h18"/>
      <path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/>
      <path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>
    </svg>`,exitChildMode:I`
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/>
      <polyline points="10 17 15 12 10 7"/>
      <line x1="15" y1="12" x2="3" y2="12"/>
    </svg>`},Xe={select:"V",move:"M",hand:"H",pencil:"B",eraser:"E",line:"L",rectangle:"U",circle:"C",triangle:"T",diamond:"D",pentagon:"P",hexagon:"K",star:"A",heart:"J",arrow:"Q",fill:"G",stamp:"S",text:"X",crop:"R",eyedropper:"I"},ta=new Map(Object.entries(Xe).map(([e,t])=>[t.toLowerCase(),e]));function ea(e){return ta.get(e.toLowerCase())}const ft={select:"Select",move:"Move",pencil:"Pencil",eraser:"Eraser",line:"Line",rectangle:"Rectangle",circle:"Circle",triangle:"Triangle",diamond:"Diamond",pentagon:"Pentagon",hexagon:"Hexagon",star:"Star",heart:"Heart",arrow:"Arrow",fill:"Fill",stamp:"Stamp",text:"Text",hand:"Hand (Pan)",crop:"Crop",eyedropper:"Eyedropper"},js=["pencil","eraser","rectangle","circle","triangle","fill"],sa=new Set(js),Ie=120,De=8,Te=1600;function rs(e,t=Ie){return Number.isFinite(e)?Math.max(De,Math.min(Te,e)):t}const _e=["rectangle","circle","line","triangle","diamond","pentagon","hexagon","star","heart","arrow"];function vt(e){return _e.includes(e)}function tt(e,t){t&&e.fill(),e.stroke()}function ia(e,t,s,i,a,o){const r=s+a/2,n=i+o/2,c=a/2,l=o/2;for(let h=0;h<t;h++){const d=-Math.PI/2+h*Math.PI*2/t,p=r+Math.cos(d)*c,f=n+Math.sin(d)*l;h===0?e.moveTo(p,f):e.lineTo(p,f)}e.closePath()}function ns(e,t,s,i,a,o,r,n){const c=Math.min(s.x,i.x),l=Math.min(s.y,i.y),h=Math.abs(i.x-s.x),d=Math.abs(i.y-s.y);switch(e.save(),e.strokeStyle=a,e.lineWidth=n,e.lineCap="round",e.lineJoin="round",r&&(e.fillStyle=o),t){case"line":e.beginPath(),e.moveTo(s.x,s.y),e.lineTo(i.x,i.y),e.stroke();break;case"rectangle":{e.beginPath(),e.rect(c,l,h,d),tt(e,r);break}case"circle":{const p=c+h/2,f=l+d/2;e.beginPath(),e.ellipse(p,f,h/2,d/2,0,0,Math.PI*2),tt(e,r);break}case"triangle":{e.beginPath(),e.moveTo(c+h/2,l),e.lineTo(c+h,l+d),e.lineTo(c,l+d),e.closePath(),tt(e,r);break}case"diamond":{e.beginPath(),e.moveTo(c+h/2,l),e.lineTo(c+h,l+d/2),e.lineTo(c+h/2,l+d),e.lineTo(c,l+d/2),e.closePath(),tt(e,r);break}case"pentagon":{e.beginPath(),ia(e,5,c,l,h,d),tt(e,r);break}case"hexagon":{e.beginPath(),e.moveTo(c+h*.25,l),e.lineTo(c+h*.75,l),e.lineTo(c+h,l+d/2),e.lineTo(c+h*.75,l+d),e.lineTo(c+h*.25,l+d),e.lineTo(c,l+d/2),e.closePath(),tt(e,r);break}case"star":{const p=c+h/2,f=l+d/2,u=h/2,_=d/2,m=.42;e.beginPath();for(let v=0;v<10;v++){const b=-Math.PI/2+v*Math.PI/5,y=v%2===0?1:m,x=p+Math.cos(b)*u*y,C=f+Math.sin(b)*_*y;v===0?e.moveTo(x,C):e.lineTo(x,C)}e.closePath(),tt(e,r);break}case"heart":{e.beginPath(),e.moveTo(c+h/2,l+d),e.bezierCurveTo(c+h*.12,l+d*.72,c,l+d*.42,c,l+d*.27),e.bezierCurveTo(c,l+d*.04,c+h*.38,l,c+h/2,l+d*.2),e.bezierCurveTo(c+h*.62,l,c+h,l+d*.04,c+h,l+d*.27),e.bezierCurveTo(c+h,l+d*.42,c+h*.88,l+d*.72,c+h/2,l+d),e.closePath(),tt(e,r);break}case"arrow":{e.beginPath(),e.moveTo(c,l+d*.3),e.lineTo(c+h*.58,l+d*.3),e.lineTo(c+h*.58,l),e.lineTo(c+h,l+d/2),e.lineTo(c+h*.58,l+d),e.lineTo(c+h*.58,l+d*.7),e.lineTo(c,l+d*.7),e.closePath(),tt(e,r);break}}e.restore()}function lt(e,t){if(typeof OffscreenCanvas<"u")return new OffscreenCanvas(e,t);const s=document.createElement("canvas");return s.width=e,s.height=t,s}function J(e,t){return e.getContext("2d",t)}function ct(e,t,s,i,a,o){a!==void 0&&o!==void 0?e.drawImage(t,s,i,a,o):e.drawImage(t,s,i)}function Ue(e,t,s,i){e.globalCompositeOperation="source-in",e.fillStyle=t,e.fillRect(0,0,s,i),e.globalCompositeOperation="source-over"}const cs=X("fan"),ls=X("splatter");function aa(e,t,s){const i=Math.max(1,e),a=lt(i,i),o=J(a),r=i/2;if(t>=1)o.fillStyle="#fff",o.beginPath(),o.arc(r,r,r,0,Math.PI*2),o.fill();else{const n=o.createRadialGradient(r,r,r*t,r,r,r);n.addColorStop(0,"rgba(255,255,255,1)"),n.addColorStop(1,"rgba(255,255,255,0)"),o.fillStyle=n,o.beginPath(),o.arc(r,r,r,0,Math.PI*2),o.fill()}return a}function oa(e,t,s){const i=Math.max(1,e),a=Math.max(1,Math.ceil(e/Math.max(1,s.aspect))),o=lt(i,a),r=J(o);if(t>=1)r.fillStyle="#fff",r.fillRect(0,0,i,a);else{r.fillStyle="#fff",r.fillRect(0,0,i,a);const n=Math.max(1,(1-t)*Math.min(i,a)*.5);r.globalCompositeOperation="destination-in";const c=r.createLinearGradient(0,0,i,0);c.addColorStop(0,"rgba(255,255,255,0)"),c.addColorStop(Math.min(.5,n/i),"rgba(255,255,255,1)"),c.addColorStop(Math.max(.5,1-n/i),"rgba(255,255,255,1)"),c.addColorStop(1,"rgba(255,255,255,0)"),r.fillStyle=c,r.fillRect(0,0,i,a);const l=r.createLinearGradient(0,0,0,a);l.addColorStop(0,"rgba(255,255,255,0)"),l.addColorStop(Math.min(.5,n/a),"rgba(255,255,255,1)"),l.addColorStop(Math.max(.5,1-n/a),"rgba(255,255,255,1)"),l.addColorStop(1,"rgba(255,255,255,0)"),r.fillStyle=l,r.fillRect(0,0,i,a),r.globalCompositeOperation="source-over"}return o}function ra(e,t,s){const i=Math.max(1,e),a=Math.max(1,Math.ceil(e/Math.max(1,s.aspect))),o=lt(i,a),r=J(o),n=a/3;if(r.beginPath(),r.moveTo(n,0),r.lineTo(i,0),r.lineTo(i-n,a),r.lineTo(0,a),r.closePath(),t>=1)r.fillStyle="#fff",r.fill();else{r.fillStyle="#fff",r.fill();const c=i/2,l=a/2,h=Math.sqrt(c*c+l*l);r.globalCompositeOperation="destination-in";const d=r.createRadialGradient(c,l,h*t,c,l,h);d.addColorStop(0,"rgba(255,255,255,1)"),d.addColorStop(1,"rgba(255,255,255,0)"),r.fillStyle=d,r.fillRect(0,0,i,a),r.globalCompositeOperation="source-over"}return o}function na(e,t,s){const i=Math.max(1,e/2),a=Math.max(1,e/Math.max(1,s.aspect)/2),o=Math.max(1,e),r=Math.max(1,Math.ceil(a*2)),n=lt(o,r),c=J(n),l=o/2,h=r/2;if(t>=1)c.fillStyle="#fff",c.beginPath(),c.ellipse(l,h,i,a,0,0,Math.PI*2),c.fill();else{c.save(),c.translate(l,h),c.scale(1,a/i);const d=c.createRadialGradient(0,0,i*t,0,0,i);d.addColorStop(0,"rgba(255,255,255,1)"),d.addColorStop(1,"rgba(255,255,255,0)"),c.fillStyle=d,c.beginPath(),c.arc(0,0,i,0,Math.PI*2),c.fill(),c.restore()}return n}function Os(e){let t=e|0;return()=>{t=t+1831565813|0;let s=Math.imul(t^t>>>15,1|t);return s=s+Math.imul(s^s>>>7,61|s)^s,((s^s>>>14)>>>0)/4294967296}}function Hs(e,t,s,i=0){const a=s.bristles??cs.bristles,r=(s.spread??cs.spread)*Math.PI/180,n=e/2,c=Math.max(1,e/8),l=lt(e,e),h=J(l),d=e/2,p=e/2,f=-Math.PI/2-r/2,u=Os(42+i*7);for(let _=0;_<a;_++){const m=a>1?_/(a-1):.5,v=f+r*m,b=n*.08*(u()-.5),y=.05*(u()-.5),x=d+Math.cos(v+y)*(n-c+b),C=p+Math.sin(v+y)*(n-c+b);if(t>=1)h.fillStyle="#fff",h.beginPath(),h.arc(x,C,c,0,Math.PI*2),h.fill();else{const k=h.createRadialGradient(x,C,c*t,x,C,c);k.addColorStop(0,"rgba(255,255,255,1)"),k.addColorStop(1,"rgba(255,255,255,0)"),h.fillStyle=k,h.beginPath(),h.arc(x,C,c,0,Math.PI*2),h.fill()}}return l}function Bs(e,t,s,i=0){const a=s.bristles??ls.bristles,o=s.spread??ls.spread,r=e/2*o,n=Math.max(1,e/10),c=lt(e,e),l=J(c),h=e/2,d=e/2,p=Os(137+i*13);for(let f=0;f<a;f++){const u=p()*Math.PI*2,_=p()*r,m=h+Math.cos(u)*_,v=d+Math.sin(u)*_,b=n*(.5+p()*.5);if(t>=1)l.fillStyle="#fff",l.beginPath(),l.arc(m,v,b,0,Math.PI*2),l.fill();else{const y=l.createRadialGradient(m,v,b*t,m,v,b);y.addColorStop(0,"rgba(255,255,255,1)"),y.addColorStop(1,"rgba(255,255,255,0)"),l.fillStyle=y,l.beginPath(),l.arc(m,v,b,0,Math.PI*2),l.fill()}}return c}const hs={round:aa,flat:oa,chisel:ra,calligraphy:na,fan:Hs,splatter:Bs},ca={fan:4,splatter:6},la=128;class ha{constructor(){this._entries=new Map,this._accessCounter=0}_buildKey(t,s,i,a){let o=`${i.shape}-${t}-${s.toFixed(2)}-${i.aspect.toFixed(1)}`;return i.bristles!=null&&(o+=`-b${i.bristles}`),i.spread!=null&&(o+=`-s${i.spread.toFixed(2)}`),a!=null&&(o+=`-v${a}`),o}get(t,s,i){const a=this._buildKey(t,s,i),o=this._entries.get(a);if(o)return o.lastUsed=++this._accessCounter,o.canvas;const r=hs[i.shape],n=r(t,s,i);return this._entries.set(a,{canvas:n,key:a,lastUsed:++this._accessCounter}),this._evictIfNeeded(),n}getVariant(t,s,i,a){const o=this._buildKey(t,s,i,a),r=this._entries.get(o);if(r)return r.lastUsed=++this._accessCounter,r.canvas;let n;return i.shape==="fan"?n=Hs(t,s,i,a):i.shape==="splatter"?n=Bs(t,s,i,a):n=hs[i.shape](t,s,i),this._entries.set(o,{canvas:n,key:o,lastUsed:++this._accessCounter}),this._evictIfNeeded(),n}_evictIfNeeded(){for(;this._entries.size>la;){let t=null,s=1/0;for(const[i,a]of this._entries)a.lastUsed<s&&(s=a.lastUsed,t=i);if(t)this._entries.delete(t);else break}}clear(){this._entries.clear(),this._accessCounter=0}}class da{constructor(){this._canvas=null,this._width=0,this._height=0}acquire(t,s){return(!this._canvas||t>this._width||s>this._height)&&(this._width=Math.max(this._width,t),this._height=Math.max(this._height,s),this._canvas=lt(this._width,this._height)),J(this._canvas).clearRect(0,0,this._width,this._height),this._canvas}commit(t,s,i,a,o,r,n=!1){if(this._canvas)if(a)t.save(),t.globalAlpha=i,t.globalCompositeOperation="destination-out",ct(t,this._canvas,0,0),t.restore();else if(n)t.save(),t.globalAlpha=i,t.globalCompositeOperation="source-over",ct(t,this._canvas,0,0),t.restore();else{const c=J(this._canvas);Ue(c,s,o,r),t.save(),t.globalAlpha=i,t.globalCompositeOperation="source-over",ct(t,this._canvas,0,0),t.restore()}}get current(){return this._canvas}}const xe=1;class pa{constructor(){this._window=[],this._count=0,this._remainder=0}reset(){this._window=[],this._count=0,this._remainder=0}addPoint(t,s,i,a,o=0){const r={x:t,y:s,pressure:i,timestamp:o};this._count++,this._window.push(r),this._window.length>4&&this._window.shift();const n=this._count;if(n===1)return this._remainder=a,[{x:t,y:s,pressure:i,speedPxPerMs:xe}];if(n===2)return this._walkLinear(this._window[0],this._window[1],a);if(n===3)return[];const[c,l,h,d]=this._window;return this._walkCatmullRom(c,l,h,d,a)}flush(t){const s=this._count,i=this._window;if(s<2)return[];if(s===2)return[];if(s===3)return this._walkLinear(i[i.length-2],i[i.length-1],t);const a=i[1],o=i[2],r=i[3],n={x:r.x+(r.x-o.x),y:r.y+(r.y-o.y),pressure:r.pressure,timestamp:r.timestamp+Math.max(0,r.timestamp-o.timestamp)};return this._walkCatmullRom(a,o,r,n,t)}_walkLinear(t,s,i){const a=s.x-t.x,o=s.y-t.y,r=Math.sqrt(a*a+o*o);if(r<.001)return[];const n=[],c=s.timestamp-t.timestamp,l=c>0?r/c:xe;let h=this._remainder;for(;h<=r;){const d=h/r;n.push({x:t.x+a*d,y:t.y+o*d,pressure:t.pressure+(s.pressure-t.pressure)*d,speedPxPerMs:l}),h+=i}return this._remainder=h-r,n}_walkCatmullRom(t,s,i,a,o){let n=0,c=s.x,l=s.y;const h=[0];for(let _=1;_<=20;_++){const m=_/20,v=Jt(t.x,s.x,i.x,a.x,m),b=Jt(t.y,s.y,i.y,a.y,m),y=Math.sqrt((v-c)**2+(b-l)**2);n+=y,h.push(n),c=v,l=b}if(n<.001)return[];const d=[],p=i.timestamp-s.timestamp,f=p>0?n/p:xe;let u=this._remainder;for(;u<=n;){let _=0;for(let x=1;x<h.length;x++)if(h[x]>=u){_=x-1;break}const m=h[_],v=h[_+1],b=v>m?(u-m)/(v-m):0,y=(_+b)/20;d.push({x:Jt(t.x,s.x,i.x,a.x,y),y:Jt(t.y,s.y,i.y,a.y,y),pressure:s.pressure+(i.pressure-s.pressure)*y,speedPxPerMs:f}),u+=o}return this._remainder=u-n,d}}function Jt(e,t,s,i,a){const o=a*a,r=o*a;return .5*(2*t+(-e+s)*a+(2*e-5*t+4*s-i)*o+(-e+3*t-3*s+i)*r)}function Ce(e){return e<=.04045?e/12.92:Math.pow((e+.055)/1.055,2.4)}function Me(e){return e<=.0031308?e*12.92:1.055*Math.pow(e,1/2.4)-.055}function ds(e,t,s){const i=Ce(e),a=Ce(t),o=Ce(s),r=Math.cbrt(.4122214708*i+.5363325363*a+.0514459929*o),n=Math.cbrt(.2119034982*i+.6806995451*a+.1073969566*o),c=Math.cbrt(.0883024619*i+.2817188376*a+.6299787005*o);return{L:.2104542553*r+.793617785*n-.0040720468*c,a:1.9779984951*r-2.428592205*n+.4505937099*c,b:.0259040371*r+.7827717662*n-.808675766*c}}function ua(e,t,s){const i=e+.3963377774*t+.2158037573*s,a=e-.1055613458*t-.0638541728*s,o=e-.0894841775*t-1.291485548*s,r=i*i*i,n=a*a*a,c=o*o*o;return{r:Math.max(0,Math.min(1,Me(4.0767416621*r-3.3077115913*n+.2309699292*c))),g:Math.max(0,Math.min(1,Me(-1.2684380046*r+2.6097574011*n-.3413193965*c))),b:Math.max(0,Math.min(1,Me(-.0041960863*r-.7034186147*n+1.707614701*c)))}}function ps(e){const t=parseInt(e.slice(1,7),16);return{r:(t>>16&255)/255,g:(t>>8&255)/255,b:(t&255)/255}}function fa(e){const t=Math.round(e.r*255),s=Math.round(e.g*255),i=Math.round(e.b*255);return`#${(1<<24|t<<16|s<<8|i).toString(16).slice(1)}`}function _a(e,t,s){if(s<=0)return e;if(s>=1)return t;const i=ps(e),a=ps(t),o=ds(i.r,i.g,i.b),r=ds(a.r,a.g,a.b),n=ua(o.L+(r.L-o.L)*s,o.a+(r.a-o.a)*s,o.b+(r.b-o.b)*s);return fa(n)}function ma(e,t){return{distanceTraveled:0,remainingPaint:1,originalColor:e,currentColor:e,stampCount:0,layerSnapshot:t,prevRotation:0}}function ga(e,t){if(e.depletion<=0)return 1;const s=Math.max(0,1-t.distanceTraveled/Math.max(1,e.depletionLength)*e.depletion);return t.remainingPaint=s,s}const va=1;function ba(e,t,s){if(e.buildup<=0)return t;const a=1-Math.min(1,Math.max(0,s/va));return Math.min(1,t*(1+e.buildup*a*3))}const ya=48;function wa(e,t,s,i=6){const a=e.width,o=e.height,r=e.data,n=Math.round(t),c=Math.round(s),l=Math.max(1,Math.round(i)),h=l*l,d=Math.max(1,Math.ceil((2*l+1)/ya));let p=0,f=0,u=0,_=0,m=0;const v=Math.max(0,n-l),b=Math.min(a-1,n+l),y=Math.max(0,c-l),x=Math.min(o-1,c+l);for(let E=y;E<=x;E+=d){const H=E-c;for(let B=v;B<=b;B+=d){const it=B-n;if(it*it+H*H>h)continue;const Q=(E*a+B)*4,at=r[Q+3];at<2||(p+=r[Q]*at,f+=r[Q+1]*at,u+=r[Q+2]*at,_+=at,m++)}}if(m===0||_===0)return{color:"#000000",alpha:0};const C=Math.round(p/_),k=Math.round(f/_),P=Math.round(u/_),R=Math.round(_/m);return{color:`#${(1<<24|C<<16|k<<8|P).toString(16).slice(1)}`,alpha:R}}function xa(e,t,s,i,a=6){if(e.wetness<=0||!t.layerSnapshot)return;const o=wa(t.layerSnapshot,s,i,Math.max(4,a));if(o.alpha<10){t.currentColor=t.originalColor;return}const r=o.alpha/255;t.currentColor=_a(t.originalColor,o.color,e.wetness*r)}const Ca=.25;function Ma(e,t,s,i){if(i.orientation==="fixed"||!t)return i.angle*Math.PI/180;const a=e.x-t.x,o=e.y-t.y;return a*a+o*o<Ca?s:Math.atan2(o,a)+i.angle*Math.PI/180}class Ys{constructor(){this._tipCache=new ha,this._bufferPool=new da,this._smoother=new pa,this._descriptor=null,this._color="",this._eraser=!1,this._colorMode=!1,this._docWidth=0,this._docHeight=0,this._lastMappedPressure=.5,this._inkState=null,this._prevStamp=null,this._variantCounter=0,this._snapshotCaptured=!1,this._tintCanvas=null,this._tintW=0,this._tintH=0,this._dirtyMinX=1/0,this._dirtyMinY=1/0,this._dirtyMaxX=-1/0,this._dirtyMaxY=-1/0}begin(t,s,i,a,o){this._descriptor={...t,tip:{...t.tip},ink:{...t.ink}},this._color=i?"":s.length===9?s.slice(0,7):s,this._eraser=i,this._colorMode=!i&&t.ink.wetness>0,this._docWidth=a,this._docHeight=o,this._bufferPool.acquire(a,o),this._smoother.reset(),this._prevStamp=null,this._variantCounter=0,this._snapshotCaptured=!1,this._dirtyMinX=1/0,this._dirtyMinY=1/0,this._dirtyMaxX=-1/0,this._dirtyMaxY=-1/0,this._inkState=ma(this._color,null)}stroke(t,s,i,a,o=0){if(!this._descriptor||!this._inkState)return;const r=this._descriptor;let n=i;if(!Number.isFinite(i)||i<=0){if(r.pressureSize||r.pressureOpacity){this._smoother.reset(),this._prevStamp=null;return}n=1}this._colorMode&&!this._snapshotCaptured&&a&&(this._inkState.layerSnapshot=a.getImageData(0,0,this._docWidth,this._docHeight),this._snapshotCaptured=!0);const c=Mi[r.pressureCurve],l=c(n);this._lastMappedPressure=l;const h=r.pressureSize?Math.max(1,r.size*l):r.size,d=Math.max(1,r.spacing*h),p=this._smoother.addPoint(t,s,l,d,o);this._stampPoints(p)}_stampPoints(t){if(!this._descriptor||!this._inkState)return;const s=this._descriptor,i=s.ink,a=this._inkState,o=this._bufferPool.current;if(!o)return;const r=J(o),n=ca[s.tip.shape]??0;for(const c of t){let l=0;if(this._prevStamp){const C=c.x-this._prevStamp.x,k=c.y-this._prevStamp.y;l=Math.sqrt(C*C+k*k),a.distanceTraveled+=l}a.stampCount++;const h=ga(i,a);if(h<=0){this._prevStamp=c;continue}const d=s.pressureOpacity?s.flow*c.pressure:s.flow,p=ba(i,d,c.speedPxPerMs),f=s.pressureSize?Math.max(1,s.size*c.pressure):s.size;xa(i,a,c.x,c.y,f/2);const u=ki(f);let _;if(n>0){const C=this._variantCounter%n;_=this._tipCache.getVariant(u,s.hardness,s.tip,C),this._variantCounter++}else _=this._tipCache.get(u,s.hardness,s.tip);const m=Ma(c,this._prevStamp,a.prevRotation,s.tip);a.prevRotation=m;const v=Math.min(1,p*h),b=_.width,y=_.height,x=Math.sqrt(b*b+y*y)/2+1;if(c.x-x<this._dirtyMinX&&(this._dirtyMinX=c.x-x),c.y-x<this._dirtyMinY&&(this._dirtyMinY=c.y-x),c.x+x>this._dirtyMaxX&&(this._dirtyMaxX=c.x+x),c.y+x>this._dirtyMaxY&&(this._dirtyMaxY=c.y+x),this._colorMode){(b>this._tintW||y>this._tintH)&&(this._tintW=Math.max(this._tintW,b),this._tintH=Math.max(this._tintH,y),this._tintCanvas=lt(this._tintW,this._tintH));const C=J(this._tintCanvas);C.globalCompositeOperation="source-over",C.clearRect(0,0,this._tintW,this._tintH),ct(C,_,0,0),Ue(C,a.currentColor,b,y),r.globalAlpha=v,r.globalCompositeOperation="source-over",m!==0?(r.save(),r.translate(Math.round(c.x),Math.round(c.y)),r.rotate(m),ct(r,this._tintCanvas,-b/2,-y/2,b,y),r.restore()):ct(r,this._tintCanvas,Math.round(c.x-b/2),Math.round(c.y-y/2),b,y)}else r.globalAlpha=v,r.globalCompositeOperation="source-over",m!==0?(r.save(),r.translate(Math.round(c.x),Math.round(c.y)),r.rotate(m),ct(r,_,-b/2,-y/2,b,y),r.restore()):ct(r,_,Math.round(c.x-b/2),Math.round(c.y-y/2),b,y);this._prevStamp=c}r.globalAlpha=1}commit(t){if(!this._descriptor)return!1;const s=this._descriptor.pressureSize?Math.max(1,this._descriptor.size*this._lastMappedPressure):this._descriptor.size,i=Math.max(1,this._descriptor.spacing*s),a=this._smoother.flush(i);return a.length>0&&this._stampPoints(a),this._bufferPool.commit(t,this._color,this._descriptor.opacity,this._eraser,this._docWidth,this._docHeight,this._colorMode),this._descriptor=null,this._inkState=null,!0}cancel(){this._descriptor=null,this._inkState=null,this._smoother.reset()}getDirtyBounds(){if(this._dirtyMaxX<this._dirtyMinX)return null;const t=Math.max(0,Math.floor(this._dirtyMinX)),s=Math.max(0,Math.floor(this._dirtyMinY)),i=Math.min(this._docWidth,Math.ceil(this._dirtyMaxX))-t,a=Math.min(this._docHeight,Math.ceil(this._dirtyMaxY))-s;return i<=0||a<=0?null:{x:t,y:s,w:i,h:a}}getStrokePreview(){return!this._descriptor||!this._bufferPool.current?null:{canvas:this._bufferPool.current,eraser:this._eraser,opacity:this._descriptor.opacity,color:this._colorMode?null:this._color,bounds:this.getDirtyBounds()}}}const ka=96,Sa=100,G=new Map,Ht=new Map,Tt=new Map;function us(e,t){const s=G.get(e);for(s&&s!==t&&URL.revokeObjectURL(s),G.delete(e),G.set(e,t);G.size>Sa;){const i=G.entries().next().value;if(!i)break;G.delete(i[0]),URL.revokeObjectURL(i[1])}return t}async function Pa(e){const t=await createImageBitmap(e);try{const s=Math.min(1,ka/Math.max(t.width,t.height)),i=document.createElement("canvas");return i.width=Math.max(1,Math.round(t.width*s)),i.height=Math.max(1,Math.round(t.height*s)),i.getContext("2d").drawImage(t,0,0,i.width,i.height),URL.createObjectURL(await Ye(i))}finally{t.close()}}async function $a(e,t){const s=G.get(t.id);if(s)return us(t.id,s);const i=Ht.get(t.id);if(i)return i;const a=Tt.get(t.id)??0,o=(async()=>{const r=await e.blobs.get(t.blobRef),n=await Pa(r);if((Tt.get(t.id)??0)!==a)throw URL.revokeObjectURL(n),new Error("Stamp thumbnail was removed while loading.");return us(t.id,n)})();Ht.set(t.id,o);try{return await o}finally{Ht.get(t.id)===o&&(Ht.delete(t.id),G.has(t.id)||Tt.delete(t.id))}}function Ia(e){const t=G.get(e);t&&URL.revokeObjectURL(t),G.delete(e),Ht.has(e)?Tt.set(e,(Tt.get(e)??0)+1):Tt.delete(e)}var Da=Object.defineProperty,Ta=Object.getOwnPropertyDescriptor,U=(e,t,s,i)=>{for(var a=i>1?void 0:i?Ta(t,s):t,o=e.length-1,r;o>=0;o--)(r=e[o])&&(a=(i?r(t,s,a):r(a))||a);return i&&a&&Da(t,s,a),a};const Ra=[{label:"800 × 600",width:800,height:600},{label:"1024 × 768",width:1024,height:768},{label:"1280 × 720 (HD)",width:1280,height:720},{label:"1920 × 1080 (Full HD)",width:1920,height:1080},{label:"2560 × 1440 (QHD)",width:2560,height:1440},{label:"A4 Portrait (794 × 1123)",width:794,height:1123},{label:"A4 Landscape (1123 × 794)",width:1123,height:794},{label:"Square 1024",width:1024,height:1024}],Ea=["#000000","#ffffff","#ff0000","#ff6600","#ffcc00","#33cc33","#0099ff","#6633ff","#cc33cc","#996633","#ff9999","#ffcc99","#ffff99","#99ff99","#99ccff","#cc99ff","#cccccc","#666666"];let O=class extends F{constructor(){super(...arguments),this._aspectLock=!1,this._recentStamps=[],this._stampBusy=!1,this._stampMessage="",this._stampMessageError=!1,this._projectDropdownOpen=!1,this._advancedOpen=!1,this._brushDropdownOpen=!1,this._previewCache=new Map,this._customPreviewKey="",this._customPreviewUrl="",this._previewCanvas=null,this._previewSampleCanvas=null,this._previewStrokeCanvas=null,this._previewEngine=new Ys,this._renamingProjectId=null,this._newProjectName="Untitled",this._newProjectWidth="800",this._newProjectHeight="600",this._thumbUrls=new Map,this._lastProjectId=null,this._stampLoadVersion=0,this._ctx=new gt(this,{context:Lt,subscribe:!0}),this._storageCtx=new gt(this,{context:Ls,subscribe:!0}),this._serviceCtx=new gt(this,{context:zs,subscribe:!0}),this._onDocumentClick=e=>{if(this._projectDropdownOpen){const t=e.composedPath(),s=this.shadowRoot?.querySelector(".project-dropdown-wrap");s&&!t.includes(s)&&this._closeDropdown()}},this._onBrushDropdownOutsideClick=e=>{const t=e.composedPath(),s=this.shadowRoot?.querySelector(".brush-dropdown-wrap");s&&!t.includes(s)&&this._closeBrushDropdown()}}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback()}willUpdate(){this.toggleAttribute("mobile",this._ctx.value?.isMobile??!1);const e=this._ctx.value?.currentProject?.id??null;e&&e!==this._lastProjectId&&(this._lastProjectId=e,this._recentStamps=[],this._thumbUrls.clear(),this._stampMessage="",this._stampMessageError=!1,this._loadStamps(e))}disconnectedCallback(){super.disconnectedCallback(),this._closeDropdown(),document.removeEventListener("click",this._onBrushDropdownOutsideClick),this._stampLoadVersion++,this._thumbUrls.clear()}async _loadStamps(e){const t=this._storageCtx.value;if(!t)return;const s=++this._stampLoadVersion;try{const i=await t.stamps.list(e);if(this._lastProjectId!==e||s!==this._stampLoadVersion)return;this._recentStamps=i;const a=new Set(i.map(o=>o.id));for(const o of this._thumbUrls.keys())a.has(o)||this._thumbUrls.delete(o);for(const o of i){if(this._lastProjectId!==e||s!==this._stampLoadVersion)return;this._thumbUrls.has(o.id)||(this._thumbUrls.set(o.id,await $a(t,o)),this._lastProjectId===e&&s===this._stampLoadVersion&&(this._recentStamps=[...i]))}this._lastProjectId===e&&s===this._stampLoadVersion&&(this._recentStamps=i)}catch(i){this._lastProjectId===e&&s===this._stampLoadVersion&&(this._stampMessage=i instanceof Error?i.message:"Could not load recent stamps.",this._stampMessageError=!0)}}_onStrokeColor(e){this.ctx.setStrokeColor(e.target.value)}_onFillColor(e){this.ctx.setFillColor(e.target.value)}_onBrushSize(e){this.ctx.setBrushSize(Number(e.target.value))}_onStampSize(e){this.ctx.setStampSize(Number(e.target.value))}_onUseFill(e){this.ctx.setUseFill(e.target.checked)}_uploadStamp(){if(!this._ctx.value?.currentProject?.id)return;const t=document.createElement("input");t.type="file",t.accept="image/*",t.multiple=!0,t.onchange=async()=>{const s=t.files;if(!s||s.length===0)return;const i=this._ctx.value?.currentProject?.id;if(!i)return;const a=this._serviceCtx.value;if(!a)return;const o=10*1024*1024,r=4096;let n=null,c=0,l=0;this._stampBusy=!0,this._stampMessage=`Importing ${s.length===1?s[0].name:`${s.length} images`}…`,this._stampMessageError=!1;try{for(const h of s){if(this._ctx.value?.currentProject?.id!==i)break;if(h.size>o){l++;continue}try{const d=await createImageBitmap(h);if(d.width>r||d.height>r){l++,d.close();continue}d.close()}catch{l++;continue}n=await a.addStamp(i,h),c++}await this._loadStamps(i),n&&this._lastProjectId===i&&(this._stampBusy=!1,await this._selectStamp(n,!1)),this._stampMessage=c>0?`${c} ${c===1?"stamp":"stamps"} added${l?`; ${l} skipped (10 MB / 4096 px limit).`:"."}`:"No stamps were added. Use a valid image up to 10 MB and 4096 px per side.",this._stampMessageError=c===0}catch(h){this._stampMessage=h instanceof Error?h.message:"Could not add the stamp.",this._stampMessageError=!0}finally{this._stampBusy=!1}},t.click()}async _selectStamp(e,t=!0){const s=this._storageCtx.value,i=this._ctx.value?.currentProject?.id??null;if(!(!s||this._stampBusy||!i||e.projectId!==i||this._lastProjectId!==i)){this._stampBusy=!0,t&&(this._stampMessage="Loading stamp…",this._stampMessageError=!1);try{const a=await s.blobs.get(e.blobRef),o=URL.createObjectURL(a);let r;try{r=await new Promise((n,c)=>{const l=new Image;l.onload=()=>n(l),l.onerror=()=>c(new Error("The stamp image could not be decoded.")),l.src=o})}finally{URL.revokeObjectURL(o)}if(this._lastProjectId!==i||this._ctx.value?.currentProject?.id!==i)return;this.ctx.setStampImage(r,e.id),t&&(this._stampMessage="Stamp selected. Click the canvas to place it.")}catch(a){this._stampMessage=a instanceof Error?a.message:"Could not load the stamp.",this._stampMessageError=!0}finally{this._stampBusy=!1}}}async _deleteStamp(e,t){t.stopPropagation();const s=this._ctx.value?.currentProject?.id;if(!s||e.projectId!==s||this._lastProjectId!==s)return;const i=this._storageCtx.value;if(i)try{if(await i.stamps.delete(e.id),Ia(e.id),this._ctx.value?.currentProject?.id!==s||this._lastProjectId!==s)return;this.ctx.state.activeStampId===e.id&&this.ctx.setStampImage(null,null),await this._loadStamps(s),this._stampMessage="Stamp removed.",this._stampMessageError=!1}catch(a){this._stampMessage=a instanceof Error?a.message:"Could not remove the stamp.",this._stampMessageError=!0}}_closeDropdown(){this._projectDropdownOpen&&(this._projectDropdownOpen=!1,document.removeEventListener("click",this._onDocumentClick))}_toggleProjectDropdown(){this._projectDropdownOpen?this._closeDropdown():(this._projectDropdownOpen=!0,document.addEventListener("click",this._onDocumentClick))}_onSelectProject(e){this._closeDropdown(),this.ctx.switchProject(e)}_onNewProject(){this._closeDropdown(),this._newProjectName="Untitled",this._newProjectWidth="800",this._newProjectHeight="600",this.shadowRoot?.querySelector(".new-project-dialog")?.showModal(),this.updateComplete.then(()=>{const t=this.shadowRoot?.querySelector(".new-project-name-input");t&&(t.focus(),t.select())})}_cancelNewProject(){this.shadowRoot?.querySelector(".new-project-dialog")?.close()}_confirmNewProject(){const e=this._newProjectName.trim()||"Untitled",t=parseInt(this._newProjectWidth),s=parseInt(this._newProjectHeight);if(!t||!s||t<=0||s<=0||t>8192||s>8192)return;this.shadowRoot?.querySelector(".new-project-dialog")?.close(),this.ctx.createProject(e,t,s)}_selectNewProjectPreset(e){this._newProjectWidth=String(e.width),this._newProjectHeight=String(e.height)}_onNewProjectKeydown(e){e.stopPropagation(),e.key==="Enter"&&this._confirmNewProject()}_onDeleteProject(e,t){e.stopPropagation(),confirm("Delete this project? This cannot be undone.")&&(this._closeDropdown(),this.ctx.deleteProject(t))}_startRename(e,t){e.stopPropagation(),this._renamingProjectId=t,this.updateComplete.then(()=>{const s=this.shadowRoot?.querySelector(".project-rename-input");s&&(s.focus(),s.select())})}_onRenameKeydown(e,t){e.stopPropagation(),e.key==="Enter"?this._commitRename(e,t):e.key==="Escape"&&(this._renamingProjectId=null)}_commitRename(e,t){if(this._renamingProjectId!==t)return;const i=e.target.value.trim();i&&this.ctx.renameProject(t,i),this._renamingProjectId=null}_showsShapeOptions(){const e=this.ctx.state.activeTool;return vt(e)&&e!=="line"}_generatePreview(e,t=!1){const s=`${t?"eraser":"paint"}:${e.id}`,i=this._previewCache.get(s);if(i)return i;const a=this._generateDescriptorPreview(e.descriptor,t);return this._previewCache.set(s,a),a}_generateCustomPreview(e,t=!1){const s=`${t?"eraser":"paint"}:${JSON.stringify(e)}`;return s===this._customPreviewKey?this._customPreviewUrl:(this._customPreviewKey=s,this._customPreviewUrl=this._generateDescriptorPreview(e,t),this._customPreviewUrl)}_generateDescriptorPreview(e,t=!1){const a=this._previewCanvas??=document.createElement("canvas");a.width!==160&&(a.width=160),a.height!==48&&(a.height=48);const o=a.getContext("2d");o.globalAlpha=1,o.globalCompositeOperation="source-over",o.clearRect(0,0,160,48),o.fillStyle="#2a2a2a",o.fillRect(0,0,160,48);const r=Math.max(2,Math.min(18,2+Math.log2(Math.max(1,e.size))*2.5)),n={...e,size:r,tip:{...e.tip},ink:{...e.ink}},c=8,l=r/2+2,h=Math.max(2,(48/2-l)*.8),d=48/2,p=160-c*2,f=this._previewSampleCanvas??=document.createElement("canvas");f.width!==160&&(f.width=160),f.height!==48&&(f.height=48);const u=f.getContext("2d");u.globalAlpha=1,u.globalCompositeOperation="source-over",u.clearRect(0,0,160,48),u.fillStyle="#ef6c57",u.fillRect(0,0,160/2,48),u.fillStyle="#5b8cf7",u.fillRect(160/2,0,160/2,48);const _=this._previewStrokeCanvas??=document.createElement("canvas");_.width!==160&&(_.width=160),_.height!==48&&(_.height=48);const m=_.getContext("2d");m.globalAlpha=1,m.globalCompositeOperation="source-over",m.clearRect(0,0,160,48),t&&(m.fillStyle="#cccccc",m.fillRect(0,0,160,48)),this._previewEngine.begin(n,"#cccccc",t,160,48);let v=0;for(let b=c;b<=160-c;b+=1){const y=(b-c)/p,x=d+h*Math.sin(y*Math.PI*2),C=.4+.6*Math.sin(y*Math.PI);v+=y>.35&&y<.65?4:1;const k=!t&&n.ink.wetness>0?u:void 0;this._previewEngine.stroke(b,x,C,k,v)}return this._previewEngine.commit(m),o.drawImage(_,0,0),a.toDataURL()}_toggleBrushDropdown(){this._brushDropdownOpen=!this._brushDropdownOpen,this._brushDropdownOpen?requestAnimationFrame(()=>{document.addEventListener("click",this._onBrushDropdownOutsideClick)}):document.removeEventListener("click",this._onBrushDropdownOutsideClick)}_closeBrushDropdown(){this._brushDropdownOpen=!1,document.removeEventListener("click",this._onBrushDropdownOutsideClick)}_selectPreset(e){this.ctx.selectPreset(e),this._closeBrushDropdown()}_renderTransformSettings(){const e=this._ctx.value?.getTransformValues();if(!e)return g`<div class="section"><label>No transform active</label></div>`;const{x:t,y:s,width:i,height:a,rotation:o,skewX:r,skewY:n,flipH:c,flipV:l}=e,h=(p,f)=>this._ctx.value?.setTransformValue(p,f),d=(p,f)=>u=>{const _=u.target.value,m=parseFloat(_);if(!isNaN(m))if(p==="width"&&this._aspectLock&&i!==0){const v=a/i;h("width",m),h("height",Math.round(m*v*10)/10)}else if(p==="height"&&this._aspectLock&&a!==0){const v=i/a;h("height",m),h("width",Math.round(m*v*10)/10)}else h(p,m)};return g`
      <div class="transform-section">
        <label>Position</label>
        <div class="transform-row">
          <span class="transform-suffix">X</span>
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(t*10)/10)}
            aria-label="X position" @change=${d("x")} />
          <span class="transform-suffix">Y</span>
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(s*10)/10)}
            aria-label="Y position" @change=${d("y")} />
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Size</label>
        <div class="transform-row">
          <span class="transform-suffix">W</span>
          <input class="transform-input" type="number" step="0.1" min="1"
            .value=${String(Math.round(i*10)/10)}
            aria-label="Width" @change=${d("width")} />
          <button
            class="aspect-lock-btn ${this._aspectLock?"active":""}"
            title="Lock aspect ratio"
            @click=${()=>{this._aspectLock=!this._aspectLock}}
          >
            <svg viewBox="0 0 10 14" width="10" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              ${this._aspectLock?g`<rect x="1" y="5" width="8" height="8" rx="1"/><path d="M3 5V3.5a2 2 0 1 1 4 0V5"/>`:g`<rect x="1" y="5" width="8" height="8" rx="1"/><path d="M3 5V3.5a2 2 0 1 1 4 0V4" stroke-dasharray="2 1"/>`}
            </svg>
          </button>
          <span class="transform-suffix">H</span>
          <input class="transform-input" type="number" step="0.1" min="1"
            .value=${String(Math.round(a*10)/10)}
            aria-label="Height" @change=${d("height")} />
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Rotation</label>
        <div class="transform-row">
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(o*10)/10)}
            aria-label="Rotation in degrees" @change=${d("rotation","°")} />
          <span class="transform-suffix">°</span>
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Skew</label>
        <div class="transform-row">
          <span class="transform-suffix">X</span>
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(r*10)/10)}
            aria-label="Horizontal skew in degrees" @change=${d("skewX","°")} />
          <span class="transform-suffix">°</span>
          <span class="transform-suffix" style="margin-left:0.25rem;">Y</span>
          <input class="transform-input" type="number" step="0.1"
            .value=${String(Math.round(n*10)/10)}
            aria-label="Vertical skew in degrees" @change=${d("skewY","°")} />
          <span class="transform-suffix">°</span>
        </div>
      </div>
      <div class="separator"></div>
      <div class="transform-section">
        <label>Flip</label>
        <div class="transform-row">
          <button
            class="flip-btn ${c?"active":""}"
            title="Flip Horizontal"
            @click=${()=>h("flipH",!0)}
          >
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M8 2v12M2 5l4 3-4 3M14 5l-4 3 4 3"/>
            </svg>
          </button>
          <button
            class="flip-btn ${l?"active":""}"
            title="Flip Vertical"
            @click=${()=>h("flipV",!0)}
          >
            <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M2 8h12M5 2l3 4 3-4M5 14l3-4 3 4"/>
            </svg>
          </button>
        </div>
      </div>
    `}render(){if(!this._ctx.value)return g``;const e=this.ctx.state,{strokeColor:t,fillColor:s,useFill:i,activeTool:a,stampImage:o,stampSize:r,brush:n}=e,c=$i(n.tip.shape),l=X(n.tip.shape);if(a==="select")return this._ctx.value.transformActive?this._renderTransformSettings():g`
        <div class="section" style="padding:16px;color:#888;font-size:12px;text-align:center;line-height:1.5;">
          Draw a selection to transform, or press
          <kbd style="background:#333;padding:1px 5px;border-radius:3px;font-size:11px;">${navigator.platform?.startsWith("Mac")?"⌘":"Ctrl"}+T</kbd>
          to transform the entire layer.
        </div>
      `;const h=n.size,d=this.ctx.isMobile;return g`
      ${d?"":g`
        ${this.ctx.embedded?S:g`
        <div class="section project-section">
          <div class="project-dropdown-wrap">
            <button class="project-name-btn" @click=${this._toggleProjectDropdown}>
              ${this.ctx.currentProject?.name??"Untitled"}
              <span class="dropdown-arrow">&#9662;</span>
            </button>
            ${this._projectDropdownOpen?g`
              <div class="project-dropdown">
                ${this.ctx.projectList.map(p=>g`
                  <div class="project-item ${p.id===this.ctx.currentProject?.id?"active":""}">
                    ${this._renamingProjectId===p.id?g`
                      <input
                        class="project-rename-input"
                        aria-label="Project name"
                        .value=${p.name}
                        @keydown=${f=>this._onRenameKeydown(f,p.id)}
                        @blur=${f=>this._commitRename(f,p.id)}
                      />
                    `:g`
                      <span class="project-item-name" @click=${()=>this._onSelectProject(p.id)}>
                        ${p.name}
                      </span>
                      <button class="project-item-action" title="Rename" @click=${f=>this._startRename(f,p.id)}>&#9998;</button>
                      <button class="project-item-action delete" title="Delete" @click=${f=>this._onDeleteProject(f,p.id)}>&#10005;</button>
                    `}
                  </div>
                `)}
                <div class="project-dropdown-divider"></div>
                <button class="project-new-btn" @click=${this._onNewProject}>+ New Project</button>
              </div>
            `:""}
          </div>
        </div>
        <div class="separator"></div>
        `}

        <div class="section" style="color:#aaa;font-size:0.75rem;">
          ${this.ctx.state.documentWidth} \u00d7 ${this.ctx.state.documentHeight}
        </div>
        <div class="separator"></div>
      `}

      ${vt(a)?g`
        <div class="section">
          <label>Shape</label>
          <div class="shape-picker" role="group" aria-label="Shape">
            ${_e.map(p=>g`
              <button
                class="shape-option ${a===p?"active":""}"
                data-shape=${p}
                title=${`${ft[p]} (${Xe[p]})`}
                aria-label=${`Select ${ft[p]} shape`}
                aria-pressed=${a===p?"true":"false"}
                @click=${()=>this.ctx.setTool(p)}
              >${Ot[p]}</button>
            `)}
          </div>
        </div>
        <div class="separator"></div>
      `:S}

      ${a!=="eraser"&&a!=="stamp"?g`
      <div class="section">
        <label>Color</label>
        <input
          type="color"
          .value=${t}
          @input=${this._onStrokeColor}
          title="Stroke color"
          aria-label="Stroke color"
        />
        <div class="color-grid">
          ${Ea.map(p=>g`
              <button
                class="color-swatch ${t===p?"active":""}"
                style="background:${p}"
                title=${p}
                @click=${()=>this.ctx.setStrokeColor(p)}
              ></button>
            `)}
        </div>
      </div>

      <div class="separator"></div>
      `:S}

      <div class="section">
        <label>${a==="stamp"?"Stamp size":"Size"}</label>
        <input
          type="range"
          min=${a==="stamp"?De:1}
          max=${a==="stamp"?Te:150}
          aria-label=${a==="stamp"?"Stamp size":"Brush size"}
          .value=${String(a==="stamp"?r:h)}
          @input=${a==="stamp"?this._onStampSize:this._onBrushSize}
        />
        ${a==="stamp"?g`
              <input
                class="stamp-size-input"
                type="number"
                min=${De}
                max=${Te}
                step="1"
                aria-label="Stamp size in pixels"
                title="Stamp size in pixels"
                .value=${String(r)}
                @change=${this._onStampSize}
              />
            `:g`<span class="size-value">${h}</span>`}
      </div>

      ${a==="pencil"||a==="eraser"?g`
        <div class="separator"></div>
        <div class="section">
          <div class="brush-dropdown-wrap">
            <button class="brush-dropdown-btn" @click=${()=>this._toggleBrushDropdown()}>
              <img src=${e.isPresetModified?this._generateCustomPreview(n,a==="eraser"):this._generatePreview(et.find(p=>p.id===e.activePreset)??et[0],a==="eraser")} alt="" />
              <span>${(et.find(p=>p.id===e.activePreset)??et[0]).name}${e.isPresetModified?" *":""}</span>
              <span class="chevron">&#9660;</span>
            </button>
            ${this._brushDropdownOpen?g`
              <div class="brush-dropdown-panel">
                ${et.map(p=>g`
                  <button
                    class="brush-dropdown-item ${e.activePreset===p.id&&!e.isPresetModified?"active":""}"
                    @click=${()=>this._selectPreset(p.id)}
                  >
                    <img src=${this._generatePreview(p,a==="eraser")} alt="" />
                    <span>${p.name}</span>
                  </button>
                `)}
              </div>
            `:S}
          </div>
        </div>
        <div class="separator"></div>
        <div class="section">
          <label>Opacity</label>
          <input type="range" aria-label="Brush opacity" min="0" max="100" .value=${String(Math.round(n.opacity*100))}
            @input=${p=>this.ctx.setBrush({opacity:Number(p.target.value)/100})} />
          <span class="size-value">${Math.round(n.opacity*100)}%</span>
        </div>
        <div class="section">
          <label>Flow</label>
          <input type="range" aria-label="Brush flow" min="1" max="100" .value=${String(Math.round(n.flow*100))}
            @input=${p=>this.ctx.setBrush({flow:Number(p.target.value)/100})} />
          <span class="size-value">${Math.round(n.flow*100)}%</span>
        </div>
        <div class="section">
          <label>Hardness</label>
          <input type="range" aria-label="Brush hardness" min="0" max="100" .value=${String(Math.round(n.hardness*100))}
            @input=${p=>this.ctx.setBrush({hardness:Number(p.target.value)/100})} />
          <span class="size-value">${Math.round(n.hardness*100)}%</span>
        </div>
        <div class="section">
          <label>Spacing</label>
          <input type="range" min="5" max="100" aria-label="Spacing" .value=${String(Math.round(n.spacing*100))}
            @input=${p=>this.ctx.setBrush({spacing:Number(p.target.value)/100})} />
          <span class="size-value">${Math.round(n.spacing*100)}%</span>
        </div>
        <div class="separator"></div>
        <div class="section">
          <label class="checkbox-label" title="Affects pressure-sensitive pen or stylus input; mouse input uses full pressure.">
            <input type="checkbox" .checked=${n.pressureSize}
              @change=${p=>this.ctx.setBrush({pressureSize:p.target.checked})} />
            Stylus Size
          </label>
        </div>
        <div class="section">
          <label class="checkbox-label" title="Affects pressure-sensitive pen or stylus input; mouse input uses full pressure.">
            <input type="checkbox" .checked=${n.pressureOpacity}
              @change=${p=>this.ctx.setBrush({pressureOpacity:p.target.checked})} />
            Stylus Opacity
          </label>
        </div>
        ${n.pressureSize||n.pressureOpacity?g`
        <div class="section">
          <label title="Maps pressure-sensitive pen or stylus input; mouse input uses full pressure.">Stylus Curve</label>
          <select class="font-select" aria-label="Stylus curve" .value=${n.pressureCurve}
            @change=${p=>this.ctx.setBrush({pressureCurve:p.target.value})}>
            <option value="linear">Linear</option>
            <option value="light">Light</option>
            <option value="heavy">Heavy</option>
          </select>
        </div>
        `:S}
        <div class="separator"></div>
        ${this._advancedOpen?g`
          <div class="section" style="flex-wrap:wrap;gap:0.5rem;">
            <label style="flex-basis:100%;cursor:pointer;" @click=${()=>{this._advancedOpen=!1}}>Advanced &#9650;</label>
            <div class="section">
              <label>Tip</label>
              <div class="pill-row">
                ${["round","flat","chisel","calligraphy","fan","splatter"].map(p=>g`
                  <button
                    class="pill-btn ${n.tip.shape===p?"active":""}"
                    @click=${()=>{n.tip.shape!==p&&this.ctx.setBrushTip(X(p))}}
                  >${p.charAt(0).toUpperCase()+p.slice(1)}</button>
                `)}
              </div>
            </div>
            ${c.aspect?g`
            <div class="section">
              <label>Aspect</label>
              <input type="range" min="1" max="6" step="0.5" .value=${String(n.tip.aspect)}
                @input=${p=>this.ctx.setBrushTip({aspect:Number(p.target.value)})} />
              <span class="size-value">${n.tip.aspect}</span>
            </div>
            `:S}
            ${c.rotation?g`
            <div class="section">
              <label>${n.tip.orientation==="direction"?"Offset":"Angle"}</label>
              <input type="range" min="0" max="360" .value=${String(n.tip.angle)}
                @input=${p=>this.ctx.setBrushTip({angle:Number(p.target.value)})} />
              <span class="size-value">${n.tip.angle}&deg;</span>
            </div>
            <div class="section">
              <label>Orient</label>
              <select class="font-select" aria-label="Tip orientation" .value=${n.tip.orientation}
                @change=${p=>this.ctx.setBrushTip({orientation:p.target.value})}>
                <option value="fixed">Fixed</option>
                <option value="direction">Direction</option>
              </select>
            </div>
            `:S}
            ${c.bristles?g`
              <div class="section">
                <label>Bristles</label>
                <input type="range" min="1" max="20" .value=${String(n.tip.bristles??l.bristles)}
                  @input=${p=>this.ctx.setBrushTip({bristles:Number(p.target.value)})} />
                <span class="size-value">${n.tip.bristles??l.bristles}</span>
              </div>
              <div class="section">
                <label>Spread</label>
                <input type="range" min="0" max="200" .value=${String(Math.round((n.tip.spread??l.spread)*(n.tip.shape==="fan"?1:100)))}
                  @input=${p=>{const f=Number(p.target.value);this.ctx.setBrushTip({spread:n.tip.shape==="fan"?f:f/100})}} />
                <span class="size-value">${n.tip.shape==="fan"?n.tip.spread??l.spread:Math.round((n.tip.spread??l.spread)*100)+"%"}</span>
              </div>
            `:S}
            <div class="section">
              <label>Depletion</label>
              <input type="range" min="0" max="100" .value=${String(Math.round(n.ink.depletion*100))}
                @input=${p=>this.ctx.setBrushInk({depletion:Number(p.target.value)/100})} />
              <span class="size-value">${Math.round(n.ink.depletion*100)}%</span>
            </div>
            ${n.ink.depletion>0?g`
              <div class="section">
                <label>Depl. Len</label>
                <input type="range" min="100" max="2000" .value=${String(n.ink.depletionLength)}
                  @input=${p=>this.ctx.setBrushInk({depletionLength:Number(p.target.value)})} />
                <span class="size-value">${n.ink.depletionLength}px</span>
              </div>
            `:S}
            ${n.flow<1||n.pressureOpacity?g`
            <div class="section">
              <label>Buildup</label>
              <input type="range" min="0" max="100" .value=${String(Math.round(n.ink.buildup*100))}
                @input=${p=>this.ctx.setBrushInk({buildup:Number(p.target.value)/100})} />
              <span class="size-value">${Math.round(n.ink.buildup*100)}%</span>
            </div>
            `:S}
            ${a!=="eraser"?g`
            <div class="section">
              <label>Wetness</label>
              <input type="range" min="0" max="100" .value=${String(Math.round(n.ink.wetness*100))}
                @input=${p=>this.ctx.setBrushInk({wetness:Number(p.target.value)/100})} />
              <span class="size-value">${Math.round(n.ink.wetness*100)}%</span>
            </div>
            `:S}
          </div>
        `:g`
          <div class="section">
            <label style="cursor:pointer;" @click=${()=>{this._advancedOpen=!0}}>Advanced &#9660;</label>
          </div>
        `}
      `:S}

      ${a==="eyedropper"?g`
        <div class="section">
          <label class="checkbox-label">
            <input type="checkbox" .checked=${e.eyedropperSampleAll}
              @change=${p=>this.ctx.setEyedropperSampleAll(p.target.checked)} />
            Sample all layers
          </label>
        </div>
      `:S}

      ${this._showsShapeOptions()?g`
            <div class="separator"></div>
            <div class="section">
              <label class="checkbox-label">
                <input type="checkbox" .checked=${i} @change=${this._onUseFill} />
                Fill
              </label>
              ${i?g`
                    <input
                      type="color"
                      .value=${s}
                      @input=${this._onFillColor}
                      title="Fill color"
                    />
                  `:""}
            </div>
          `:""}

      ${a==="crop"?g`
            <div class="separator"></div>
            <div class="section">
              <label>Ratio</label>
              <select
                .value=${this.ctx.state.cropAspectRatio}
                @change=${p=>this.ctx.setCropAspectRatio(p.target.value)}
                style="background:#444;color:#ddd;border:1px solid #555;border-radius:0.25rem;padding:0.25rem 0.375rem;font-size:0.8125rem;cursor:pointer;"
              >
                <option value="free">Free</option>
                <option value="1:1">1:1</option>
                <option value="4:3">4:3</option>
                <option value="3:2">3:2</option>
                <option value="16:9">16:9</option>
                <option value="3:4">3:4</option>
                <option value="2:3">2:3</option>
                <option value="9:16">9:16</option>
              </select>
            </div>
          `:""}

      ${a==="text"?g`
            <div class="separator"></div>
            <div class="section">
              <label>Font</label>
              <select
                class="font-select"
                aria-label="Font"
                .value=${this.ctx.state.fontFamily}
                @change=${p=>this.ctx.setFontFamily(p.target.value)}
              >
                <option value="sans-serif">Sans-serif</option>
                <option value="serif">Serif</option>
                <option value="monospace">Monospace</option>
                <option value="Arial">Arial</option>
                <option value="Georgia">Georgia</option>
                <option value="Courier New">Courier New</option>
                <option value="Verdana">Verdana</option>
                <option value="Times New Roman">Times New Roman</option>
              </select>
            </div>
            <div class="section">
              <label>Size</label>
              <input
                class="font-size-input"
                type="number"
                min="8"
                max="200"
                .value=${String(this.ctx.state.fontSize)}
                @change=${p=>this.ctx.setFontSize(Number(p.target.value))}
              />
            </div>
            <div class="section">
              <button
                class="font-toggle ${this.ctx.state.fontBold?"active":""}"
                title="Bold"
                @click=${()=>this.ctx.setFontBold(!this.ctx.state.fontBold)}
              ><strong>B</strong></button>
              <button
                class="font-toggle ${this.ctx.state.fontItalic?"active":""}"
                title="Italic"
                @click=${()=>this.ctx.setFontItalic(!this.ctx.state.fontItalic)}
              ><em>I</em></button>
            </div>
          `:""}

      ${a==="stamp"?g`
            <div class="stamp-line">
              <button
                class="stamp-btn"
                @click=${this._uploadStamp}
                title="Upload one or more stamp images"
                aria-label="Upload one or more stamp images"
                ?disabled=${this._stampBusy}
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
                  <polyline points="17 8 12 3 7 8"/>
                  <line x1="12" y1="3" x2="12" y2="15"/>
                </svg>
              </button>
              ${this._recentStamps.length>0?g`
                    <div class="stamp-row">
                      ${this._recentStamps.map((p,f)=>g`
                          <div class="stamp-thumb-wrap">
                            <button
                              class="stamp-thumb ${e.activeStampId===p.id?"active":""}"
                              aria-label=${`Select recent stamp ${f+1}`}
                              aria-pressed=${e.activeStampId===p.id?"true":"false"}
                              title=${`Select recent stamp ${f+1}`}
                              @click=${()=>this._selectStamp(p)}
                              ?disabled=${this._stampBusy}
                            >
                              <img
                                src=${this._thumbUrls.get(p.id)??""}
                                alt=""
                                loading="lazy"
                                decoding="async"
                              />
                            </button>
                            <button
                              class="stamp-delete"
                              aria-label=${`Delete recent stamp ${f+1}`}
                              title=${`Delete recent stamp ${f+1}`}
                              @click=${u=>this._deleteStamp(p,u)}
                              ?disabled=${this._stampBusy}
                            >&times;</button>
                          </div>
                        `)}
                    </div>
                  `:""}
            </div>
            <div
              class="stamp-help ${this._stampMessageError?"error":""}"
              role="status"
              aria-live="polite"
            >
              ${this._stampMessage||(o?"Click the canvas to place the selected stamp. Enter accepts; Escape cancels.":"Choose a recent stamp or upload an image, then click the canvas.")}
            </div>
          `:""}

      ${this.ctx.saving?g`
            <div class="saving-indicator" role="img" aria-label="Saving">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 12a9 9 0 1 1-6.219-8.56"></path>
              </svg>
            </div>
          `:""}

      <dialog class="new-project-dialog" @keydown=${this._onNewProjectKeydown}>
        <p class="dialog-title">New Project</p>
        <div class="dialog-field">
          <label>Name</label>
          <input
            type="text"
            class="new-project-name-input"
            .value=${this._newProjectName}
            @input=${p=>{this._newProjectName=p.target.value}}
          />
        </div>
        <div class="dialog-field">
          <label>Canvas Size</label>
          <div class="dialog-presets">
            ${Ra.map(p=>g`
              <button
                class="dialog-preset-btn ${String(p.width)===this._newProjectWidth&&String(p.height)===this._newProjectHeight?"active":""}"
                @click=${()=>this._selectNewProjectPreset(p)}
              >${p.label}</button>
            `)}
          </div>
          <div class="dialog-size-row">
            <input
              class="dialog-size-input"
              type="number"
              min="1"
              max="8192"
              .value=${this._newProjectWidth}
              @input=${p=>{this._newProjectWidth=p.target.value}}
            />
            <span>\u00d7</span>
            <input
              class="dialog-size-input"
              type="number"
              min="1"
              max="8192"
              .value=${this._newProjectHeight}
              @input=${p=>{this._newProjectHeight=p.target.value}}
            />
          </div>
        </div>
        <div class="dialog-actions">
          <button class="dialog-cancel-btn" @click=${this._cancelNewProject}>Cancel</button>
          <button class="dialog-create-btn" @click=${this._confirmNewProject}>Create</button>
        </div>
      </dialog>
    `}};O.styles=yt`
    :host {
      display: flex;
      align-items: center;
      background: #333;
      padding: 0.375rem 1rem;
      column-gap: 1rem;
      row-gap: 0.25rem;
      color: #ddd;
      font-family: system-ui, sans-serif;
      font-size: 0.8125rem;
      flex-wrap: wrap;
      min-height: 2.75rem;
      position: relative;
    }

    .section {
      display: flex;
      align-items: center;
      gap: 0.5rem;
    }

    label {
      color: #aaa;
      white-space: nowrap;
    }

    .color-grid {
      display: flex;
      flex-wrap: wrap;
      gap: 0.1875rem;
      max-width: 15rem;
    }

    .color-swatch {
      width: 1.5rem;
      height: 1.5rem;
      border-radius: 0.1875rem;
      border: 0.125rem solid transparent;
      cursor: pointer;
      padding: 0;
      box-sizing: border-box;
    }

    .color-swatch:hover {
      border-color: #888;
    }

    .color-swatch.active {
      border-color: #5b8cf7;
    }

    input[type="color"] {
      width: 1.75rem;
      height: 1.75rem;
      border: none;
      border-radius: 0.25rem;
      padding: 0;
      cursor: pointer;
      background: none;
    }

    input[type="range"] {
      width: 6.25rem;
      accent-color: #5b8cf7;
    }

    .size-value {
      min-width: 1.5rem;
      text-align: center;
    }

    .stamp-size-input {
      width: 4.5rem;
      min-height: 2rem;
      box-sizing: border-box;
      border: 1px solid #555;
      border-radius: 0.25rem;
      background: #222;
      color: #ddd;
      padding: 0.25rem 0.375rem;
    }

    .checkbox-label {
      display: flex;
      align-items: center;
      gap: 0.25rem;
      cursor: pointer;
      color: #aaa;
    }

    .checkbox-label input {
      accent-color: #5b8cf7;
    }

    .stamp-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      background: #5b8cf7;
      color: white;
      border: none;
      border-radius: 6px;
      cursor: pointer;
      padding: 6px;
      flex-shrink: 0;
    }

    .stamp-btn svg {
      width: 15px;
      height: 15px;
    }

    .stamp-btn:hover {
      background: #4a7be6;
    }

    .stamp-btn:disabled,
    .stamp-thumb:disabled,
    .stamp-delete:disabled {
      opacity: 0.55;
      cursor: wait;
    }

    .separator {
      width: 0.0625rem;
      height: 1.5rem;
      background: #555;
    }

    .stamp-row {
      display: flex;
      gap: 0.25rem;
      overflow-x: auto;
      overflow-y: hidden;
      padding: 0.125rem 0;
      align-items: center;
      scrollbar-width: none;
    }

    .stamp-row::-webkit-scrollbar {
      display: none;
    }

    .stamp-thumb-wrap {
      position: relative;
      flex-shrink: 0;
      overflow: hidden;
      border-radius: 0.25rem;
    }

    .stamp-thumb {
      width: 2.75rem;
      height: 2.75rem;
      border-radius: 0.25rem;
      border: 0.125rem solid transparent;
      object-fit: contain;
      background: #222;
      cursor: pointer;
      display: block;
      padding: 0;
      box-sizing: border-box;
    }

    .stamp-thumb img {
      width: 100%;
      height: 100%;
      object-fit: contain;
      display: block;
    }

    .stamp-thumb:hover {
      border-color: #888;
    }

    .stamp-thumb.active {
      border-color: #5b8cf7;
    }

    .stamp-delete {
      position: absolute;
      top: -0.125rem;
      right: -0.125rem;
      width: 1.5rem;
      height: 1.5rem;
      border-radius: 50%;
      background: #444;
      color: #fff;
      border: none;
      font-size: 0.875rem;
      line-height: 1.5rem;
      text-align: center;
      cursor: pointer;
      padding: 0;
      z-index: 1;
    }

    @media (hover: hover) and (pointer: fine) {
      .stamp-delete {
        opacity: 0;
        pointer-events: none;
      }

      .stamp-thumb-wrap:hover .stamp-delete,
      .stamp-thumb-wrap:focus-within .stamp-delete {
        opacity: 1;
        pointer-events: auto;
      }
    }

    @media (hover: none), (pointer: coarse) {
      .stamp-btn {
        width: 2.75rem;
        height: 2.75rem;
      }

      .stamp-thumb-wrap {
        display: flex;
        gap: 0.125rem;
        overflow: visible;
      }

      .stamp-delete {
        position: static;
        width: 2.75rem;
        height: 2.75rem;
        border-radius: 0.25rem;
        line-height: 2.75rem;
      }
    }

    .stamp-delete:hover {
      background: #e55;
    }

    .stamp-line {
      flex-basis: 100%;
      display: flex;
      align-items: center;
      gap: 0.5rem;
      margin-left: -0.5rem;
    }

    .stamp-help {
      flex-basis: 100%;
      color: #aaa;
      font-size: 0.75rem;
      margin-left: -0.5rem;
    }

    .stamp-help.error {
      color: #ff8a8a;
    }

    .project-section {
      position: relative;
    }

    .project-dropdown-wrap {
      position: relative;
    }

    .project-name-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.5rem;
      cursor: pointer;
      font-size: 0.8125rem;
      display: flex;
      align-items: center;
      gap: 0.25rem;
      max-width: 12rem;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .project-name-btn:hover {
      background: #555;
    }

    .dropdown-arrow {
      font-size: 0.625rem;
      opacity: 0.7;
    }

    .project-dropdown {
      position: absolute;
      top: 100%;
      left: 0;
      margin-top: 0.25rem;
      background: #3a3a3a;
      border: 1px solid #555;
      border-radius: 0.375rem;
      min-width: 14rem;
      max-height: 20rem;
      overflow-y: auto;
      z-index: 100;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
      padding: 0.25rem 0;
    }

    .project-item {
      display: flex;
      align-items: center;
      padding: 0.375rem 0.5rem;
      gap: 0.25rem;
    }

    .project-item.active {
      background: #4a4a4a;
    }

    .project-item-name {
      flex: 1;
      cursor: pointer;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      padding: 0.125rem 0;
    }

    .project-item-name:hover {
      color: #fff;
    }

    .project-item-action {
      background: none;
      border: none;
      color: #888;
      cursor: pointer;
      padding: 0.125rem 0.25rem;
      font-size: 0.75rem;
      border-radius: 0.125rem;
      line-height: 1;
      width: auto;
      height: auto;
    }

    .project-item-action:hover {
      color: #ddd;
      background: #555;
    }

    .project-item-action.delete:hover {
      color: #ff6666;
    }

    .project-rename-input {
      flex: 1;
      background: #2a2a2a;
      border: 1px solid #5b8cf7;
      border-radius: 0.1875rem;
      color: #ddd;
      padding: 0.125rem 0.25rem;
      font-size: 0.8125rem;
      outline: 2px solid transparent;
    }

    .project-dropdown-divider {
      height: 1px;
      background: #555;
      margin: 0.25rem 0;
    }

    .project-new-btn {
      display: block;
      width: 100%;
      text-align: left;
      background: none;
      border: none;
      color: #5b8cf7;
      cursor: pointer;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      height: auto;
      border-radius: 0;
    }

    .project-new-btn:hover {
      background: #4a4a4a;
      color: #7aa8ff;
    }

    .saving-indicator {
      position: absolute;
      top: 0.75rem;
      right: 1rem;
      color: #888;
      width: 1.25rem;
      height: 1.25rem;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .saving-indicator svg {
      width: 100%;
      height: 100%;
      animation: spin 1s linear infinite;
    }

    @keyframes spin {
      100% { transform: rotate(360deg); }
    }

    dialog {
      background: #3a3a3a;
      border: 1px solid #555;
      border-radius: 0.5rem;
      color: #ddd;
      padding: 1.25rem;
      min-width: 18rem;
      box-shadow: 0 8px 24px rgba(0,0,0,0.5);
      font-family: system-ui, sans-serif;
      font-size: 0.8125rem;
    }

    dialog::backdrop {
      background: rgba(0,0,0,0.5);
    }

    .dialog-title {
      font-size: 1rem;
      font-weight: 600;
      margin: 0 0 1rem 0;
    }

    .dialog-field {
      display: flex;
      flex-direction: column;
      gap: 0.25rem;
      margin-bottom: 0.75rem;
    }

    .dialog-field label {
      color: #aaa;
      font-size: 0.75rem;
    }

    .dialog-field input[type="text"] {
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      outline: 2px solid transparent;
    }

    .dialog-field input[type="text"]:focus {
      border-color: #5b8cf7;
    }

    .dialog-presets {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
      margin-bottom: 0.5rem;
    }

    .dialog-preset-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.5rem;
      cursor: pointer;
      font-size: 0.75rem;
      height: auto;
    }

    .dialog-preset-btn:hover {
      background: #555;
    }

    .dialog-preset-btn.active {
      border-color: #5b8cf7;
      color: #5b8cf7;
    }

    .dialog-size-row {
      display: flex;
      align-items: center;
      gap: 0.375rem;
    }

    .dialog-size-input {
      width: 5rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.375rem 0.5rem;
      font-size: 0.8125rem;
      text-align: center;
      outline: 2px solid transparent;
    }

    .dialog-size-input:focus {
      border-color: #5b8cf7;
    }

    .dialog-actions {
      display: flex;
      justify-content: flex-end;
      gap: 0.5rem;
      margin-top: 1rem;
    }

    .dialog-cancel-btn {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.375rem 0.75rem;
      cursor: pointer;
      font-size: 0.8125rem;
    }

    .dialog-cancel-btn:hover {
      background: #555;
    }

    .dialog-create-btn {
      background: #5b8cf7;
      color: white;
      border: none;
      border-radius: 0.25rem;
      padding: 0.375rem 0.75rem;
      cursor: pointer;
      font-size: 0.8125rem;
    }

    .dialog-create-btn:hover {
      background: #4a7be6;
    }

    .font-select {
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.375rem;
      font-size: 0.8125rem;
      cursor: pointer;
    }

    .font-size-input {
      width: 3.5rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      padding: 0.25rem 0.375rem;
      font-size: 0.8125rem;
      text-align: center;
    }

    .font-toggle {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.75rem;
      height: 1.75rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      cursor: pointer;
      font-size: 0.8125rem;
      padding: 0;
    }

    .font-toggle.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    /* Brush preset dropdown */
    .brush-dropdown-wrap {
      position: relative;
      width: 100%;
    }

    .brush-dropdown-btn {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      width: 100%;
      padding: 0.25rem 0.5rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.375rem;
      cursor: pointer;
      color: #ddd;
      font-size: 0.8125rem;
    }

    .brush-dropdown-btn:hover {
      border-color: #888;
    }

    .brush-dropdown-btn img {
      width: 80px;
      height: 24px;
      border-radius: 0.125rem;
      object-fit: cover;
    }

    .brush-dropdown-btn .chevron {
      margin-left: auto;
      font-size: 0.625rem;
      color: #888;
    }

    .brush-dropdown-panel {
      position: absolute;
      top: 100%;
      left: 0;
      right: 0;
      z-index: 50;
      background: #333;
      border: 1px solid #555;
      border-radius: 0.375rem;
      margin-top: 0.125rem;
      max-height: 300px;
      overflow-y: auto;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
    }

    .brush-dropdown-item {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      width: 100%;
      padding: 0.375rem 0.5rem;
      background: none;
      border: none;
      cursor: pointer;
      color: #ccc;
      font-size: 0.8125rem;
      text-align: left;
    }

    .brush-dropdown-item:hover {
      background: #444;
    }

    .brush-dropdown-item.active {
      background: #3a3a4a;
      color: #5b8cf7;
    }

    .brush-dropdown-item img {
      width: 120px;
      height: 36px;
      border-radius: 0.25rem;
      object-fit: cover;
      overflow: hidden;
    }

    /* Pill-button shape selector */
    .pill-row {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
    }

    .pill-btn {
      background: #444;
      color: #bbb;
      border: 1px solid #555;
      border-radius: 1rem;
      padding: 0.15rem 0.5rem;
      cursor: pointer;
      font-size: 0.75rem;
      white-space: nowrap;
      height: auto;
    }

    .pill-btn:hover {
      background: #555;
      color: #ddd;
    }

    .pill-btn.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    .shape-picker {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25rem;
    }

    .shape-option {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 2rem;
      height: 2rem;
      padding: 0.35rem;
      border: 1px solid #555;
      border-radius: 0.375rem;
      background: #444;
      color: #bbb;
      cursor: pointer;
    }

    .shape-option:hover {
      background: #555;
      color: #fff;
    }

    .shape-option.active {
      background: #5b8cf7;
      border-color: #77a1ff;
      color: #fff;
    }

    .shape-option svg {
      width: 1.125rem;
      height: 1.125rem;
    }

    /* Dimmed control */

    /* Transform numeric panel */
    .transform-section {
      display: flex;
      flex-direction: column;
      gap: 0.2rem;
    }

    .transform-section > label {
      font-size: 0.6875rem;
      color: #888;
      white-space: nowrap;
    }

    .transform-row {
      display: flex;
      align-items: center;
      gap: 0.25rem;
    }

    .transform-input {
      width: 4rem;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #ddd;
      padding: 0.2rem 0.3rem;
      font-size: 0.75rem;
      text-align: center;
      outline: 2px solid transparent;
      -moz-appearance: textfield;
    }

    .transform-input::-webkit-inner-spin-button,
    .transform-input::-webkit-outer-spin-button {
      -webkit-appearance: none;
      margin: 0;
    }

    .transform-input:focus {
      border-color: #5b8cf7;
    }

    .transform-suffix {
      color: #888;
      font-size: 0.75rem;
    }

    .flip-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.75rem;
      height: 1.75rem;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 0.25rem;
      cursor: pointer;
      font-size: 0.75rem;
      padding: 0;
    }

    .flip-btn:hover {
      background: #555;
    }

    .flip-btn.active {
      background: #5b8cf7;
      color: #fff;
      border-color: #5b8cf7;
    }

    .aspect-lock-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 1.25rem;
      height: 1.25rem;
      background: none;
      border: 1px solid #555;
      border-radius: 0.25rem;
      color: #888;
      cursor: pointer;
      padding: 0;
      font-size: 0.6875rem;
      flex-shrink: 0;
    }

    .aspect-lock-btn:hover {
      border-color: #888;
      color: #ddd;
    }

    .aspect-lock-btn.active {
      border-color: #5b8cf7;
      color: #5b8cf7;
    }

    /* ── Inside mobile popover ─────────────────── */
    :host([mobile]) {
      flex-direction: column;
      align-items: flex-start;
      padding: 0;
      min-height: 0;
      background: transparent;
      touch-action: manipulation;
    }

    :host([mobile]) .section {
      width: 100%;
      min-width: 0;
    }

    :host([mobile]) .separator {
      display: none;
    }

    :host([mobile]) input[type="range"] {
      flex: 1;
      min-width: 0;
      width: 0;
    }

    :host([mobile]) .stamp-size-input {
      min-height: 2.75rem;
    }
  `;U([M()],O.prototype,"_aspectLock",2);U([M()],O.prototype,"_recentStamps",2);U([M()],O.prototype,"_stampBusy",2);U([M()],O.prototype,"_stampMessage",2);U([M()],O.prototype,"_stampMessageError",2);U([M()],O.prototype,"_projectDropdownOpen",2);U([M()],O.prototype,"_advancedOpen",2);U([M()],O.prototype,"_brushDropdownOpen",2);U([M()],O.prototype,"_renamingProjectId",2);U([M()],O.prototype,"_newProjectName",2);U([M()],O.prototype,"_newProjectWidth",2);U([M()],O.prototype,"_newProjectHeight",2);O=U([wt("tool-settings")],O);var La=Object.defineProperty,za=Object.getOwnPropertyDescriptor,We=(e,t,s,i)=>{for(var a=i>1?void 0:i?za(t,s):t,o=e.length-1,r;o>=0;o--)(r=e[o])&&(a=(i?r(t,s,a):r(a))||a);return i&&a&&La(t,s,a),a};const ae=[..._e],$t=[["select","move","crop","hand"],["pencil","eraser"],ae,["fill","stamp","text","eyedropper"]],Aa=[{hex:"#000000",name:"Black"},{hex:"#ffffff",name:"White"},{hex:"#ff3b30",name:"Red"},{hex:"#ff9500",name:"Orange"},{hex:"#ffcc00",name:"Yellow"},{hex:"#34c759",name:"Green"},{hex:"#00c7be",name:"Teal"},{hex:"#007aff",name:"Blue"},{hex:"#5856d6",name:"Indigo"},{hex:"#af52de",name:"Purple"},{hex:"#ff2d55",name:"Pink"},{hex:"#a2845e",name:"Brown"}],ke=[{label:"S",value:4},{label:"M",value:16},{label:"L",value:40}];let Ft=class extends F{constructor(){super(...arguments),this._popoverGroup=null,this._isFullscreen=!1,this._lastToolPerGroup=new Map,this._ctx=new gt(this,{context:Lt,subscribe:!0}),this._onFullscreenChange=()=>{this._isFullscreen=!!document.fullscreenElement},this._toggleFullscreen=()=>{document.fullscreenElement?document.exitFullscreen():document.documentElement.requestFullscreen(),this._closePopover()}}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback(),document.addEventListener("fullscreenchange",this._onFullscreenChange)}disconnectedCallback(){super.disconnectedCallback(),document.removeEventListener("fullscreenchange",this._onFullscreenChange)}willUpdate(){this.toggleAttribute("mobile",this.ctx?.isMobile??!1),this.toggleAttribute("child-mode",this.ctx?.state?.childMode??!1);const e=this.ctx?.state?.activeTool;if(e){const t=$t.findIndex(s=>s.includes(e));t!==-1&&this._lastToolPerGroup.set(t,e)}this.ctx?.state?.layersPanelOpen&&this._popoverGroup!==null&&(this._popoverGroup=null)}_selectTool(e){this.ctx.setTool(e)}render(){if(!this._ctx.value)return g``;const{activeTool:e}=this.ctx.state;return this.ctx.isMobile?this._renderMobile(e):g`
      ${$t.map((t,s)=>g`
          ${s>0?g`<div class="separator"></div>`:""}
          <div class="group">
            ${t===ae?g`
                <button
                  class=${vt(e)?"active":""}
                  title="Shapes (U)"
                  aria-label="Shapes"
                  @click=${()=>this._selectTool(vt(e)?e:this._lastToolPerGroup.get(s)??_e[0])}
                >
                  ${os}
                </button>
              `:t.map(i=>g`
                <button
                  class=${e===i?"active":""}
                  title=${`${ft[i]} (${Xe[i]})`}
                  @click=${()=>this._selectTool(i)}
                >
                  ${Ot[i]}
                </button>
              `)}
          </div>
        `)}

      <div class="action-group">
        <div class="separator"></div>
        <button
          title="Undo"
          ?disabled=${!this.ctx.canUndo}
          @click=${()=>this.ctx.undo()}
        >
          ${Y.undo}
        </button>
        <button
          title="Redo"
          ?disabled=${!this.ctx.canRedo}
          @click=${()=>this.ctx.redo()}
        >
          ${Y.redo}
        </button>
        <button title="Save" @click=${()=>this.ctx.saveCanvas()}>
          ${Y.save}
        </button>
        <button title="Clear canvas" @click=${()=>this.ctx.clearCanvas()}>
          ${Y.clear}
        </button>
      </div>
    `}_closestChildSize(e){let t=ke[0].value,s=Math.abs(e-t);for(const i of ke){const a=Math.abs(e-i.value);a<s&&(s=a,t=i.value)}return t}_confirmClearCanvas(){confirm("Clear the whole drawing?")&&this.ctx.clearCanvas()}_renderChildMode(e){const t=this.ctx.state.strokeColor,s=this.ctx.state.brush.size,i=this._closestChildSize(s);return g`
      <div class="child-bar">
        <div class="child-colors">
          ${Aa.map(a=>g`
            <button
              class="child-color-btn ${t===a.hex?"active":""}"
              style="background:${a.hex}${a.hex==="#ffffff"?";box-shadow:inset 0 0 0 1px #666":""}"
              aria-label=${a.name}
              @click=${()=>this.ctx.setStrokeColor(a.hex)}
            ></button>
          `)}
          <input
            type="color"
            class="child-color-picker"
            .value=${t}
            @input=${a=>this.ctx.setStrokeColor(a.target.value)}
            title="Pick color"
          />
        </div>

        <div class="child-tools-row">
          <button
            class="child-tool-btn"
            title="Undo"
            ?disabled=${!this.ctx.canUndo}
            @click=${()=>this.ctx.undo()}
          >${Y.undo}</button>
          <button
            class="child-tool-btn"
            title="Redo"
            ?disabled=${!this.ctx.canRedo}
            @click=${()=>this.ctx.redo()}
          >${Y.redo}</button>

          <div class="child-sep"></div>

          ${js.map(a=>g`
            <button
              class="child-tool-btn ${e===a?"active":""}"
              title=${ft[a]}
              @click=${()=>this._selectTool(a)}
            >${Ot[a]}</button>
          `)}

          <div class="child-sep"></div>

          ${ke.map(a=>g`
            <button
              class="child-size-btn ${i===a.value?"active":""}"
              title="${a.label} brush"
              @click=${()=>this.ctx.setBrushSize(a.value)}
            >${a.label}</button>
          `)}

          <div class="child-sep"></div>

          <button
            class="child-tool-btn"
            title="Save"
            @click=${()=>this.ctx.saveCanvas()}
          >${Y.save}</button>
          <button
            class="child-tool-btn"
            title="Clear canvas"
            @click=${()=>this._confirmClearCanvas()}
          >${Y.clear}</button>
          <button
            class="child-tool-btn"
            title="Exit Child Mode"
            @click=${()=>this.ctx.setChildMode(!1)}
          >${Y.exitChildMode}</button>
        </div>
      </div>
    `}_renderMobile(e){return this.ctx.state.childMode?this._renderChildMode(e):g`
      <!-- Undo/Redo at left -->
      <button
        title="Undo"
        ?disabled=${!this.ctx.canUndo}
        @click=${()=>this.ctx.undo()}
      >${Y.undo}</button>
      <button
        title="Redo"
        ?disabled=${!this.ctx.canRedo}
        @click=${()=>this.ctx.redo()}
      >${Y.redo}</button>

      <div class="separator"></div>

      <!-- Tool groups: show one representative button per group -->
      ${$t.map((t,s)=>{const a=t.find(n=>n===e)??t[0],o=t.includes(e),r=t===ae;return g`
          <button
            class=${o?"active":""}
            title=${r?"Shapes":ft[a]}
            aria-label=${r?"Shapes":ft[a]}
            aria-expanded=${this._popoverGroup===s}
            aria-controls="mobile-tool-popover"
            @click=${()=>this._onMobileToolTap(t,s)}
          >${r?os:Ot[a]}</button>
        `})}

      <div class="separator"></div>

      <!-- Layers button -->
      <button
        title="Layers"
        @click=${()=>{this._closePopover(),this.ctx.toggleLayersPanel()}}
      >${g`<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>`}</button>

      <!-- More actions -->
      <button
        title="More"
        @click=${()=>this._onMobileMoreTap()}
      ><svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><circle cx="12" cy="5" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="19" r="2"/></svg></button>

      <!-- Popover -->
      ${this._popoverGroup!==null?g`
        <div class="popover-backdrop" @click=${()=>this._closePopover()}></div>
        <div class="popover" id="mobile-tool-popover">
          ${this._renderPopoverContent(e)}
        </div>
      `:""}
    `}_onMobileToolTap(e,t){const{activeTool:s}=this.ctx.state;if(e.includes(s)){const a=this._popoverGroup===t?null:t;this._popoverGroup=a,a!==null&&this.ctx.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}else this.ctx.setTool(this._lastToolPerGroup.get(t)??e[0]),this._popoverGroup=null}_onMobileMoreTap(){const e=this._popoverGroup===-1?null:-1;this._popoverGroup=e,e!==null&&this.ctx.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}_closePopover(){this._popoverGroup=null}_stopCropActionKeydown(e){e.key==="Enter"&&e.stopPropagation()}_dispatchCropShortcut(e){this.dispatchEvent(new KeyboardEvent("keydown",{key:e,bubbles:!0,composed:!0})),this._closePopover()}_renderPopoverContent(e){if(this._popoverGroup===-1)return g`
        <button class="menu-btn" @click=${()=>{const s=$t.findIndex(i=>i.includes(e));s!==-1&&this._onMobileToolTap($t[s],s)}}>Tool settings</button>
        <div class="popover-divider"></div>
        ${this.ctx.embedded?"":g`
        <span class="popover-label">Projects</span>
        <div class="project-list">
          ${this.ctx.projectList.map(s=>g`
            <button
              class="project-item ${s.id===this.ctx.currentProject?.id?"current":""}"
              @click=${()=>{this.ctx.switchProject(s.id),this._closePopover()}}
            >
              <span class="check">${s.id===this.ctx.currentProject?.id?"✓":""}</span>
              ${s.name}
            </button>
          `)}
        </div>
        <button class="menu-btn" @click=${()=>{this.ctx.createProject("Untitled",800,600),this._closePopover()}}>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
          New Project
        </button>
        <div class="popover-divider"></div>
        `}
        <button class="menu-btn" title="Save" @click=${()=>{this.ctx.saveCanvas(),this._closePopover()}}>${Y.save} Save</button>
        <button class="menu-btn" title="Clear canvas" @click=${()=>{this.ctx.clearCanvas(),this._closePopover()}}>${Y.clear} Clear</button>
        <div class="popover-divider"></div>
        <button class="menu-btn" @click=${this._toggleFullscreen}>
          ${this._isFullscreen?g`<svg viewBox="0 0 16 16" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M5 2v3H2M14 5h-3V2M11 14v-3h3M2 11h3v3"/></svg>`:g`<svg viewBox="0 0 16 16" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4"/></svg>`}
          ${this._isFullscreen?"Exit Fullscreen":"Fullscreen"}
        </button>
        <div class="popover-divider"></div>
        <button class="menu-btn" @click=${()=>{this.ctx.setChildMode(!0),this._closePopover()}}>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M8 14s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
          Child Mode
        </button>
      `;const t=$t[this._popoverGroup];return t?g`
      ${t.length>1&&t!==ae?g`
        <div class="sub-tools">
          ${t.map(s=>g`
            <button
              class=${e===s?"active":""}
              title=${ft[s]}
              @click=${()=>{this.ctx.setTool(s),this._lastToolPerGroup.set(this._popoverGroup,s)}}
            >${Ot[s]}</button>
          `)}
        </div>
      `:""}
      ${e==="crop"?g`
        <button
          class="menu-btn"
          @keydown=${this._stopCropActionKeydown}
          @click=${()=>this._dispatchCropShortcut("Enter")}
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
          Apply crop
        </button>
        <button
          class="menu-btn"
          @keydown=${this._stopCropActionKeydown}
          @click=${()=>this._dispatchCropShortcut("Escape")}
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>
          Cancel crop
        </button>
        <div class="popover-divider"></div>
      `:""}
      <tool-settings></tool-settings>
    `:g``}};Ft.styles=yt`
    :host {
      display: flex;
      flex-direction: column;
      background: #2c2c2c;
      padding: 8px;
      gap: 4px;
      width: 60px;
      box-sizing: border-box;
      overflow-x: hidden;
      overflow-y: auto;
      scrollbar-width: thin;
      scrollbar-color: #555 transparent;
    }

    .group {
      display: flex;
      flex-direction: column;
      gap: 2px;
    }

    .separator {
      height: 1px;
      background: #555;
      margin: 4px 2px;
    }

    button {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      padding: 10px;
      transition: all 0.15s ease;
    }

    button:hover {
      background: #444;
      color: #fff;
    }

    button.active {
      background: #5b8cf7;
      color: #fff;
    }

    button:disabled {
      opacity: 0.3;
      cursor: default;
    }

    button:disabled:hover {
      background: transparent;
      color: #bbb;
    }

    button svg {
      width: 20px;
      height: 20px;
    }

    .action-group {
      margin-top: auto;
      display: flex;
      flex-direction: column;
      gap: 2px;
    }

    /* ── Mobile bottom bar ─────────────────────── */
    :host([mobile]) {
      flex-direction: row;
      width: 100%;
      height: 48px;
      padding: 4px 8px;
      padding-bottom: calc(4px + env(safe-area-inset-bottom));
      align-items: center;
      justify-content: space-between;
      overflow: visible;
      border-top: 1px solid #444;
      touch-action: manipulation;
    }

    :host([mobile][child-mode]) {
      height: auto;
      padding: 8px 8px;
      padding-bottom: calc(8px + env(safe-area-inset-bottom));
      justify-content: center;
    }

    :host([mobile]) .group {
      flex-direction: row;
    }

    :host([mobile]) .separator {
      width: 1px;
      height: 24px;
      margin: 0 4px;
    }

    :host([mobile]) .action-group {
      flex-direction: row;
      margin-top: 0;
      margin-left: auto;
    }

    .popover {
      display: none;
    }

    :host([mobile]) .popover {
      display: flex;
      flex-direction: column;
      position: absolute;
      bottom: calc(52px + env(safe-area-inset-bottom));
      left: 8px;
      right: 8px;
      max-height: calc(100dvh - 72px - env(safe-area-inset-bottom));
      box-sizing: border-box;
      overflow-y: auto;
      overscroll-behavior: contain;
      background: #2c2c2c;
      border: 1px solid #555;
      border-radius: 12px;
      padding: 8px;
      gap: 8px;
      z-index: 100;
      touch-action: manipulation;
      box-shadow: 0 -4px 16px rgba(0,0,0,0.4);
    }

    .popover > * {
      flex-shrink: 0;
    }

    .popover-backdrop {
      display: none;
    }

    :host([mobile]) .popover-backdrop {
      display: block;
      position: fixed;
      inset: 0;
      z-index: 99;
    }

    .popover .sub-tools {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      padding-bottom: 8px;
      border-bottom: 1px solid #444;
    }

    .popover-divider {
      height: 1px;
      background: #444;
    }

    .popover-label {
      font-size: 0.7rem;
      color: #888;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      padding: 0 4px;
    }

    .popover .project-list {
      display: flex;
      flex-direction: column;
      gap: 2px;
      max-height: 40vh;
      overflow-y: auto;
    }

    .popover .project-item {
      display: flex;
      align-items: center;
      gap: 6px;
      width: 100%;
      padding: 8px 10px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      font-size: 0.85rem;
      text-align: left;
      cursor: pointer;
    }

    .popover .project-item:hover {
      background: #444;
    }

    .popover .project-item.current {
      background: #3a3a3a;
      color: #fff;
    }

    .popover .project-item .check {
      width: 16px;
      flex-shrink: 0;
      color: #5b8cf7;
    }

    .popover button.menu-btn {
      display: flex;
      align-items: center;
      gap: 8px;
      width: 100%;
      height: auto;
      padding: 10px;
      font-size: 0.85rem;
      text-align: left;
      border-radius: 6px;
    }

    .popover button.menu-btn svg {
      flex-shrink: 0;
    }

    /* ── Child Mode ───────────────────────────── */
    .child-bar {
      display: flex;
      flex-direction: column;
      width: 100%;
      gap: 6px;
    }

    .child-colors {
      display: flex;
      justify-content: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .child-color-btn {
      width: 36px;
      height: 36px;
      border-radius: 50%;
      border: 3px solid transparent;
      padding: 0;
      cursor: pointer;
      transition: transform 0.15s ease, border-color 0.15s ease;
    }

    .child-color-btn:hover {
      transform: scale(1.15);
    }

    .child-color-btn.active {
      border-color: #fff;
      box-shadow: 0 0 0 2px #5b8cf7;
      transform: scale(1.15);
    }

    .child-tools-row {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 4px;
      flex-wrap: wrap;
    }

    .child-tool-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 48px;
      height: 48px;
      border: none;
      border-radius: 14px;
      background: #3a3a3a;
      color: #ccc;
      cursor: pointer;
      padding: 0;
      transition: all 0.15s ease;
    }

    .child-tool-btn svg {
      width: 22px;
      height: 22px;
    }

    .child-tool-btn:hover {
      background: #555;
      color: #fff;
    }

    .child-tool-btn.active {
      background: #5b8cf7;
      color: #fff;
    }

    .child-tool-btn:disabled {
      opacity: 0.3;
      cursor: default;
    }

    .child-size-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 40px;
      height: 48px;
      border: none;
      border-radius: 14px;
      background: #3a3a3a;
      color: #ccc;
      font-size: 0.85rem;
      font-weight: 700;
      cursor: pointer;
      padding: 0;
      transition: all 0.15s ease;
    }

    .child-size-btn:hover {
      background: #555;
      color: #fff;
    }

    .child-size-btn.active {
      background: #ff9500;
      color: #fff;
    }

    .child-sep {
      width: 1px;
      height: 32px;
      background: #555;
      margin: 0 2px;
      flex-shrink: 0;
    }

    .child-color-picker {
      width: 36px;
      height: 36px;
      border: none;
      border-radius: 50%;
      padding: 0;
      cursor: pointer;
      background: none;
    }
  `;We([M()],Ft.prototype,"_popoverGroup",2);We([M()],Ft.prototype,"_isFullscreen",2);Ft=We([wt("app-toolbar")],Ft);function Ne(e,t=0){let s=null,i=null,a=0;const o=()=>{s=null,i=null,a=performance.now(),e()};return{schedule(){if(s!==null||i!==null)return;const r=t-(performance.now()-a);r<=0?s=requestAnimationFrame(o):i=setTimeout(()=>{i=null,s=requestAnimationFrame(o)},r)},cancel(){s!==null&&cancelAnimationFrame(s),i!==null&&clearTimeout(i),s=null,i=null}}}function fs(e){return Math.max(0,Math.min(1,e))}function _s(e){return e.pointerType==="mouse"?1:e.pointerType==="pen"?Number.isFinite(e.pressure)?fs(e.pressure):.5:e.pointerType==="touch"?1:e.pressure>0&&Number.isFinite(e.pressure)?fs(e.pressure):.5}function ja(e,t,s,i,a=32){const{width:o,height:r}=e.canvas,n=Math.floor(t),c=Math.floor(s);if(n<0||n>=o||c<0||c>=r)return!1;const l=e.getImageData(0,0,o,r),h=l.data,d=Ba(i),p=(c*o+n)*4,f=h[p],u=h[p+1],_=h[p+2],m=h[p+3];if(a===0&&f===d.r&&u===d.g&&_===d.b&&m===d.a)return!1;const v=Ha(o*r),b=oe,y=C=>Oa(h,C*4,f,u,_,m,a),x=[n,c];for(;x.length>0;){const C=x.pop(),k=x.pop(),P=C*o;if(v[P+k]===b||!y(P+k))continue;let R=k;for(;R>0&&v[P+R-1]!==b&&y(P+R-1);)R--;let E=k;for(;E<o-1&&v[P+E+1]!==b&&y(P+E+1);)E++;for(let H=R;H<=E;H++){const B=(P+H)*4;h[B]=d.r,h[B+1]=d.g,h[B+2]=d.b,h[B+3]=d.a,v[P+H]=b}C>0&&ms(x,v,b,y,o,C-1,R,E),C<r-1&&ms(x,v,b,y,o,C+1,R,E)}return e.putImageData(l,0,0),!0}function ms(e,t,s,i,a,o,r,n){const c=o*a;let l=!1;for(let h=r;h<=n;h++){const d=t[c+h]!==s&&i(c+h);d&&!l&&e.push(h,o),l=d}}function Oa(e,t,s,i,a,o,r){return Math.abs(e[t]-s)<=r&&Math.abs(e[t+1]-i)<=r&&Math.abs(e[t+2]-a)<=r&&Math.abs(e[t+3]-o)<=r}let Qt=new Uint16Array(0),oe=0;function Ha(e){return Qt.length<e&&(Qt=new Uint16Array(e),oe=0),++oe>65535&&(Qt.fill(0),oe=1),Qt}const te=new Map;let dt=null;function Ba(e){const t=te.get(e);if(t)return t;if(!dt){const n=document.createElement("canvas");n.width=1,n.height=1,dt=n.getContext("2d",{willReadFrequently:!0})}dt.clearRect(0,0,1,1),dt.fillStyle="#000",dt.fillStyle=e,dt.fillRect(0,0,1,1);const[s,i,a,o]=dt.getImageData(0,0,1,1).data,r={r:s,g:i,b:a,a:o};return te.size>64&&te.clear(),te.set(e,r),r}function Ya(e,t,s,i,a,o){e.save(),e.lineWidth=1,e.setLineDash([6,6]),e.strokeStyle="#ffffff",e.lineDashOffset=0,e.strokeRect(t+.5,s+.5,i,a),e.strokeStyle="#3b82f6",e.lineDashOffset=o,e.strokeRect(t+.5,s+.5,i,a),e.restore()}const Xs=8;function Xa(e,t,s,i,a){const o=Xs/a,r=o/2;e.fillStyle="rgba(0, 0, 0, 0.5)",e.fillRect(0,0,s,t.y),e.fillRect(0,t.y+t.h,s,i-t.y-t.h),e.fillRect(0,t.y,t.x,t.h),e.fillRect(t.x+t.w,t.y,s-t.x-t.w,t.h),e.strokeStyle="#ffffff",e.lineWidth=1/a,e.setLineDash([]),e.strokeRect(t.x,t.y,t.w,t.h);const n=[{cx:t.x,cy:t.y},{cx:t.x+t.w/2,cy:t.y},{cx:t.x+t.w,cy:t.y},{cx:t.x+t.w,cy:t.y+t.h/2},{cx:t.x+t.w,cy:t.y+t.h},{cx:t.x+t.w/2,cy:t.y+t.h},{cx:t.x,cy:t.y+t.h},{cx:t.x,cy:t.y+t.h/2}];e.fillStyle="#ffffff",e.strokeStyle="#3b82f6",e.lineWidth=1/a;for(const{cx:u,cy:_}of n)e.fillRect(u-r,_-r,o,o),e.strokeRect(u-r,_-r,o,o);const c=`${Math.round(t.w)} × ${Math.round(t.h)}`,l=Math.max(10,12/a);e.font=`${l}px system-ui, sans-serif`,e.textAlign="right",e.textBaseline="top";const h=t.x+t.w,d=t.y+t.h+4/a,p=e.measureText(c),f=3/a;e.fillStyle="rgba(0, 0, 0, 0.7)",e.fillRect(h-p.width-f*2,d,p.width+f*2,l+f*2),e.fillStyle="#ffffff",e.fillText(c,h-f,d+f)}function gs(e,t,s){const a=Xs/s/2,o=[{handle:"nw",cx:e.x,cy:e.y},{handle:"n",cx:e.x+e.w/2,cy:e.y},{handle:"ne",cx:e.x+e.w,cy:e.y},{handle:"e",cx:e.x+e.w,cy:e.y+e.h/2},{handle:"se",cx:e.x+e.w,cy:e.y+e.h},{handle:"s",cx:e.x+e.w/2,cy:e.y+e.h},{handle:"sw",cx:e.x,cy:e.y+e.h},{handle:"w",cx:e.x,cy:e.y+e.h/2}];for(const{handle:r,cx:n,cy:c}of o)if(t.x>=n-a&&t.x<=n+a&&t.y>=c-a&&t.y<=c+a)return r;return t.x>=e.x&&t.x<=e.x+e.w&&t.y>=e.y&&t.y<=e.y+e.h?"move":null}function Se(e){if(e==="free")return null;const t=e.split(":");if(t.length!==2)return null;const s=parseFloat(t[0]),i=parseFloat(t[1]);return!s||!i?null:s/i}function vs(e,t,s){const{x:i,y:a,w:o,h:r}=e,n=Math.abs(o),c=Math.abs(r),l=o>=0?1:-1,h=r>=0?1:-1;let d,p;s==="n"||s==="s"?(p=c,d=c*t):s==="e"||s==="w"||n/t>=c?(d=n,p=n/t):(p=c,d=c*t);const f=d*l,u=p*h,_=s==="nw"||s==="w"||s==="sw",m=s==="nw"||s==="n"||s==="ne",v=_?i+o-f:i,b=m?a+r-u:a;return{x:v,y:b,w:f,h:u}}const le=1.2;function he(e,t,s,i){const a=t.includes(" ")?`'${t}'`:t;return`${i?"italic ":""}${s?"bold ":""}${e}px ${a}`}function bs(e,t,s,i,a,o,r,n,c){if(!t)return;e.save(),e.font=he(a,o,r,n),e.fillStyle=c,e.textBaseline="top";const l=a*le,h=t.split(`
`);for(let d=0;d<h.length;d++)e.fillText(h[d],s,i+d*l);e.restore()}function ys(e,t,s,i,a,o){e.save(),e.font=he(s,i,a,o),e.textBaseline="top";const r=t.split(`
`),n=s*le,c=r.map(d=>e.measureText(d).width);let l=0;for(const d of c)d>l&&(l=d);const h=Math.max(1,r.length)*n;return e.restore(),{width:l,height:h,lineWidths:c}}const ws={size:8,hitRadius:6,shape:"square",rotationStemLength:30},Ua={size:20,hitRadius:20,shape:"circle",rotationStemLength:50},Wa=4,Na=3;function de(e){const t=e.width/2,s=e.height/2,i=e.skewX*Math.PI/180,a=e.skewY*Math.PI/180,o=Math.cos(e.rotation),r=Math.sin(e.rotation),n=(u,_)=>({a:u.a*_.a+u.c*_.b,b:u.b*_.a+u.d*_.b,c:u.a*_.c+u.c*_.d,d:u.b*_.c+u.d*_.d,e:u.a*_.e+u.c*_.f+u.e,f:u.b*_.e+u.d*_.f+u.f}),c=(u,_)=>({a:1,b:0,c:0,d:1,e:u,f:_}),l=()=>({a:o,b:r,c:-r,d:o,e:0,f:0}),h=()=>({a:1,b:0,c:Math.tan(i),d:1,e:0,f:0}),d=()=>({a:1,b:Math.tan(a),c:0,d:1,e:0,f:0}),p=()=>({a:e.scaleX,b:0,c:0,d:e.scaleY,e:0,f:0}),f=[c(e.x+t,e.y+s),l(),h(),d(),p(),c(-t,-s)].reduce(n,{a:1,b:0,c:0,d:1,e:0,f:0});return new DOMMatrix([f.a,f.b,f.c,f.d,f.e,f.f])}function Re(e,t){const s=de(t),i=s.a*s.d-s.b*s.c;return Math.abs(i)<1e-10?{x:e.x,y:e.y}:{x:(s.d*e.x-s.c*e.y+s.c*s.f-s.d*s.e)/i,y:(-s.b*e.x+s.a*e.y+s.b*s.e-s.a*s.f)/i}}function A(e,t){const s=de(t);return{x:s.a*e.x+s.c*e.y+s.e,y:s.b*e.x+s.d*e.y+s.f}}function Fa(e){const{width:t,height:s}=e;return[A({x:0,y:0},e),A({x:t,y:0},e),A({x:t,y:s},e),A({x:0,y:s},e)]}function Xt(e){return A({x:e.width/2,y:e.height/2},e)}function Va(e,t){return Math.round(e/t)*t}function xs(e){const{data:t,width:s,height:i}=e;let a=s,o=-1,r=-1,n=-1;for(let c=0;c<i;c++){const l=c*s*4+3;let h=0;for(;h<s&&t[l+h*4]===0;)h++;if(h===s)continue;o<0&&(o=c),n=c,h<a&&(a=h);let d=s-1;for(;d>r&&t[l+d*4]===0;)d--;d>r&&(r=d)}return o<0?null:{x:a,y:o,w:r-a+1,h:n-o+1}}function ee(e,t){const[s,i,a,o]=Fa(e);return[{x:s.x+t.nw.x,y:s.y+t.nw.y},{x:i.x+t.ne.x,y:i.y+t.ne.y},{x:a.x+t.se.x,y:a.y+t.se.y},{x:o.x+t.sw.x,y:o.y+t.sw.y}]}function Cs(e,t,s,i,a){const[o,r,n,c]=s,[l,h,d,p]=i;for(let f=0;f<a;f++)for(let u=0;u<a;u++){const _=u/a,m=(u+1)/a,v=f/a,b=(f+1)/a,y=ot(o,r,n,c,_,v),x=ot(o,r,n,c,m,v),C=ot(o,r,n,c,_,b),k=ot(o,r,n,c,m,b),P=ot(l,h,d,p,_,v),R=ot(l,h,d,p,m,v),E=ot(l,h,d,p,_,b),H=ot(l,h,d,p,m,b);Ms(e,t,y,x,C,P,R,E),Ms(e,t,x,k,C,R,H,E)}}function ot(e,t,s,i,a,o){const r={x:e.x+(t.x-e.x)*a,y:e.y+(t.y-e.y)*a},n={x:i.x+(s.x-i.x)*a,y:i.y+(s.y-i.y)*a};return{x:r.x+(n.x-r.x)*o,y:r.y+(n.y-r.y)*o}}function Ms(e,t,s,i,a,o,r,n){const c=i.x-s.x,l=i.y-s.y,h=a.x-s.x,d=a.y-s.y,p=r.x-o.x,f=r.y-o.y,u=n.x-o.x,_=n.y-o.y,m=c*d-h*l;if(Math.abs(m)<1e-10)return;const v=1/m,b=d*v,y=-h*v,x=-l*v,C=c*v,k=b*p+x*u,P=y*p+C*u,R=b*f+x*_,E=y*f+C*_,H=o.x-k*s.x-P*s.y,B=o.y-R*s.x-E*s.y;e.save(),e.beginPath(),e.moveTo(o.x,o.y),e.lineTo(r.x,r.y),e.lineTo(n.x,n.y),e.closePath(),e.clip(),e.setTransform(k,R,P,E,H,B),e.drawImage(t,0,0),e.restore()}function qa(e,t){return{nw:{x:0,y:0},n:{x:e/2,y:0},ne:{x:e,y:0},e:{x:e,y:t/2},se:{x:e,y:t},s:{x:e/2,y:t},sw:{x:0,y:t},w:{x:0,y:t/2}}}function Us(e){const t=qa(e.width,e.height),s={};for(const[i,a]of Object.entries(t))s[i]=A(a,e);return s}function Ws(e,t,s){const i=A({x:e.width/2,y:0},e),a=Xt(e),o=i.x-a.x,r=i.y-a.y,n=Math.sqrt(o*o+r*r);if(n<1)return i;const c=t.rotationStemLength/s;return{x:i.x+o/n*c,y:i.y+r/n*c}}function Ns(e,t,s,i){const a=Us(t),o=s.hitRadius/i;for(const[r,n]of Object.entries(a)){const c=e.x-n.x,l=e.y-n.y;if(c*c+l*l<=o*o)return r}return null}function Fs(e,t,s,i){const a=Ws(t,s,i),o=s.hitRadius/i,r=e.x-a.x,n=e.y-a.y;return r*r+n*n<=o*o}function Vs(e,t){const s=Re(e,t);return s.x>=0&&s.x<=t.width&&s.y>=0&&s.y<=t.height}function Za(e,t,s,i){const a=Us(t),o=s.size/2/i;e.save(),e.fillStyle="#ffffff",e.strokeStyle="#3b82f6",e.lineWidth=1.5/i;for(const r of Object.values(a))s.shape==="circle"?(e.beginPath(),e.arc(r.x,r.y,o,0,Math.PI*2),e.fill(),e.stroke()):(e.fillRect(r.x-o,r.y-o,o*2,o*2),e.strokeRect(r.x-o,r.y-o,o*2,o*2));e.restore()}function Ka(e,t,s,i){const a=A({x:t.width/2,y:0},t),o=Ws(t,s,i),r=(s.shape==="circle"?8:6)/i;e.save(),e.strokeStyle="#3b82f6",e.lineWidth=1.5/i,e.fillStyle="#ffffff",e.beginPath(),e.moveTo(a.x,a.y),e.lineTo(o.x,o.y),e.stroke(),e.beginPath(),e.arc(o.x,o.y,r,0,Math.PI*2),e.fill(),e.stroke(),e.restore()}function re(e,t,s){const i=A({x:e.width,y:0},e),a=Xt(e),o=i.x-a.x,r=i.y-a.y,n=Math.sqrt(o*o+r*r),c=t.shape==="circle",l=(c?38:30)/s,h=(c?22:12)/s,d=(c?48:28)/s,p=n>1?i.x+o/n*l:i.x+l,f=n>1?i.y+r/n*l:i.y-l;return{commitCenter:{x:p,y:f},cancelCenter:{x:p+d,y:f},buttonRadius:h}}function Ga(e,t,s,i){const{commitCenter:a,cancelCenter:o,buttonRadius:r}=re(t,s,i);e.save(),e.lineCap="round",e.lineJoin="round",e.fillStyle="#ffffff",e.strokeStyle="#22c55e",e.lineWidth=1.5/i,e.beginPath(),e.arc(a.x,a.y,r,0,Math.PI*2),e.fill(),e.stroke(),e.strokeStyle="#16a34a",e.lineWidth=1.5/i,e.beginPath();const n=r*.4;e.moveTo(a.x-n,a.y+n*.1),e.lineTo(a.x-n*.15,a.y+n*.65),e.lineTo(a.x+n,a.y-n*.55),e.stroke(),e.fillStyle="#ffffff",e.strokeStyle="#ef4444",e.lineWidth=1.5/i,e.beginPath(),e.arc(o.x,o.y,r,0,Math.PI*2),e.fill(),e.stroke(),e.strokeStyle="#dc2626",e.lineWidth=1.5/i,e.beginPath();const c=r*.32;e.moveTo(o.x-c,o.y-c),e.lineTo(o.x+c,o.y+c),e.moveTo(o.x+c,o.y-c),e.lineTo(o.x-c,o.y+c),e.stroke(),e.restore()}function Ja(e,t,s,i){if(Fs(e,t,s,i))return"grab";const a=Ns(e,t,s,i);return a?{nw:"nwse-resize",ne:"nesw-resize",se:"nwse-resize",sw:"nesw-resize",n:"ns-resize",s:"ns-resize",e:"ew-resize",w:"ew-resize"}[a]:Vs(e,t)?"move":"crosshair"}class rt{constructor(t,s,i,a,o){this._perspectiveCorners={nw:{x:0,y:0},ne:{x:0,y:0},se:{x:0,y:0},sw:{x:0,y:0}},this._perspectiveActive=!1,this._interaction={type:"idle"},this._handleConfig=ws,this._sourceImageData=t,this._sourceRect=s,this._previewCanvas=i,this._zoom=a,this._pan=o,this._sourceCanvas=document.createElement("canvas"),this._sourceCanvas.width=t.width,this._sourceCanvas.height=t.height,this._sourceCanvas.getContext("2d").putImageData(t,0,0),this._state={x:s.x,y:s.y,width:s.w,height:s.h,rotation:0,skewX:0,skewY:0,scaleX:1,scaleY:1},this._initialState={...this._state},this.renderPreview()}get x(){return this._state.x}set x(t){this._state.x=t,this._onChange()}get y(){return this._state.y}set y(t){this._state.y=t,this._onChange()}get width(){return Math.abs(this._state.width*this._state.scaleX)}set width(t){t<=0||(this._state.scaleX=(this._state.scaleX<0?-1:1)*t/this._state.width,this._onChange())}get height(){return Math.abs(this._state.height*this._state.scaleY)}set height(t){t<=0||(this._state.scaleY=(this._state.scaleY<0?-1:1)*t/this._state.height,this._onChange())}get rotation(){return this._state.rotation*180/Math.PI}set rotation(t){this._state.rotation=t*Math.PI/180,this._onChange()}get skewX(){return this._state.skewX}set skewX(t){this._state.skewX=Math.max(-89,Math.min(89,t)),this._onChange()}get skewY(){return this._state.skewY}set skewY(t){this._state.skewY=Math.max(-89,Math.min(89,t)),this._onChange()}get flipH(){return this._state.scaleX<0}set flipH(t){const s=t,i=this._state.scaleX<0;s!==i&&(this._state.scaleX=-this._state.scaleX,this._onChange())}get flipV(){return this._state.scaleY<0}set flipV(t){const s=t,i=this._state.scaleY<0;s!==i&&(this._state.scaleY=-this._state.scaleY,this._onChange())}get perspectiveActive(){return this._perspectiveActive}setTouchMode(t){this._handleConfig=t?Ua:ws,this.renderPreview()}onPointerDown(t,s){const i=re(this._state,this._handleConfig,this._zoom);if(Math.hypot(t.x-i.commitCenter.x,t.y-i.commitCenter.y)<=i.buttonRadius||Math.hypot(t.x-i.cancelCenter.x,t.y-i.cancelCenter.y)<=i.buttonRadius)return!0;if(Fs(t,this._state,this._handleConfig,this._zoom)){const n=Xt(this._state),c=Math.atan2(t.y-n.y,t.x-n.x);return this._interaction={type:"rotating",startAngle:c,startRotation:this._state.rotation},!0}const r=Ns(t,this._state,this._handleConfig,this._zoom);return r?(s.ctrl&&(r==="nw"||r==="ne"||r==="se"||r==="sw")?(this._perspectiveActive=!0,this._interaction={type:"perspective",corner:r,startPoint:t}):s.ctrl&&(r==="n"||r==="e"||r==="s"||r==="w")?this._interaction={type:"skewing",edge:r,startPoint:t,startSkewX:this._state.skewX,startSkewY:this._state.skewY}:this._interaction={type:"resizing",handle:r,origin:{rect:{x:this._state.x,y:this._state.y,w:this._state.width,h:this._state.height},point:t}},!0):Vs(t,this._state)?(this._interaction={type:"moving",startPoint:t,startX:this._state.x,startY:this._state.y},!0):(this._interaction={type:"outside-pending",startPoint:t},!0)}onPointerMove(t,s){switch(this._interaction.type){case"moving":this._handleMove(t,s);break;case"resizing":this._handleResize(t,s);break;case"rotating":this._handleRotate(t,s);break;case"skewing":this._handleSkew(t);break;case"perspective":this._handlePerspective(t);break;case"outside-pending":{const i=t.x-this._interaction.startPoint.x,a=t.y-this._interaction.startPoint.y;if(Math.sqrt(i*i+a*a)*this._zoom>Na){const r=Xt(this._state),n=Math.atan2(this._interaction.startPoint.y-r.y,this._interaction.startPoint.x-r.x);this._interaction={type:"rotating",startAngle:n,startRotation:this._state.rotation},this._handleRotate(t,s)}break}}}onPointerUp(t){const s=re(this._state,this._handleConfig,this._zoom);if(Math.hypot(t.x-s.commitCenter.x,t.y-s.commitCenter.y)<=s.buttonRadius)return this._interaction={type:"idle"},"commit-button";if(Math.hypot(t.x-s.cancelCenter.x,t.y-s.cancelCenter.y)<=s.buttonRadius)return this._interaction={type:"idle"},"cancel-button";const o=this._interaction.type==="outside-pending"?"commit":null;return this._interaction={type:"idle"},o}_handleMove(t,s){const i=this._interaction;if(i.type!=="moving")return;let a=t.x-i.startPoint.x,o=t.y-i.startPoint.y;s.shift&&(Math.abs(a)>Math.abs(o)?o=0:a=0),this._state.x=i.startX+a,this._state.y=i.startY+o,this._onChange()}_handleResize(t,s){const i=this._interaction;if(i.type!=="resizing")return;const{handle:a,origin:o}=i,{rect:r,point:n}=o,c=Re(t,this._state),l=Re(n,this._state),h=c.x-l.x,d=c.y-l.y;let p=r.x,f=r.y,u=r.w,_=r.h;if(a.includes("e")&&(u=r.w+h),a.includes("w")&&(p=r.x+h,u=r.w-h),a.includes("s")&&(_=r.h+d),a.includes("n")&&(f=r.y+d,_=r.h-d),s.shift&&(a==="nw"||a==="ne"||a==="se"||a==="sw")){const v=r.w/r.h;Math.abs(u/_)>v?_=u/v:u=_*v}const m=Wa/this._zoom;Math.abs(u)<m&&(u=u<0?-m:m),Math.abs(_)<m&&(_=_<0?-m:m),this._state.x=p,this._state.y=f,this._state.width=Math.abs(u),this._state.height=Math.abs(_),u<0&&(this._state.scaleX=-Math.abs(this._state.scaleX)),_<0&&(this._state.scaleY=-Math.abs(this._state.scaleY)),this._onChange()}_handleRotate(t,s){const i=this._interaction;if(i.type!=="rotating")return;const a=Xt(this._state),o=Math.atan2(t.y-a.y,t.x-a.x);let r=i.startRotation+(o-i.startAngle);s.shift&&(r=Va(r,Math.PI/12)),this._state.rotation=r,this._onChange()}_handleSkew(t){const s=this._interaction;if(s.type!=="skewing")return;const i=t.x-s.startPoint.x,a=t.y-s.startPoint.y;if(s.edge==="n"||s.edge==="s"){const o=s.edge==="n"?-1:1;this._state.skewX=Math.max(-89,Math.min(89,s.startSkewX+o*i*.5))}else{const o=s.edge==="w"?-1:1;this._state.skewY=Math.max(-89,Math.min(89,s.startSkewY+o*a*.5))}this._onChange()}_handlePerspective(t){const s=this._interaction;if(s.type!=="perspective")return;const i=t.x-s.startPoint.x,a=t.y-s.startPoint.y;this._perspectiveCorners[s.corner]={x:i,y:a},this._onChange()}renderPreview(){const t=this._previewCanvas.getContext("2d"),s=this._previewCanvas.width,i=this._previewCanvas.height;t.clearRect(0,0,s,i),t.save(),t.translate(this._pan.x,this._pan.y),t.scale(this._zoom,this._zoom);const a=this._perspectiveActive?ee(this._state,this._perspectiveCorners):[A({x:0,y:0},this._state),A({x:this._state.width,y:0},this._state),A({x:this._state.width,y:this._state.height},this._state),A({x:0,y:this._state.height},this._state)];t.save(),t.lineWidth=1/this._zoom,t.setLineDash([6/this._zoom,6/this._zoom]),t.strokeStyle="#ffffff",t.lineDashOffset=0,t.beginPath(),t.moveTo(a[0].x,a[0].y);for(let o=1;o<4;o++)t.lineTo(a[o].x,a[o].y);t.closePath(),t.stroke(),t.strokeStyle="#3b82f6",t.lineDashOffset=4/this._zoom,t.beginPath(),t.moveTo(a[0].x,a[0].y);for(let o=1;o<4;o++)t.lineTo(a[o].x,a[o].y);t.closePath(),t.stroke(),t.restore(),Za(t,this._state,this._handleConfig,this._zoom),Ka(t,this._state,this._handleConfig,this._zoom),Ga(t,this._state,this._handleConfig,this._zoom),t.restore()}renderTransformed(t){if(this._perspectiveActive){const s=[{x:0,y:0},{x:this._sourceCanvas.width,y:0},{x:this._sourceCanvas.width,y:this._sourceCanvas.height},{x:0,y:this._sourceCanvas.height}],i=ee(this._state,this._perspectiveCorners),a=8,o=i.map(f=>f.x),r=i.map(f=>f.y),n=Math.floor(Math.min(...o)),c=Math.floor(Math.min(...r)),l=Math.ceil(Math.max(...o)),h=Math.ceil(Math.max(...r)),d=l-n,p=h-c;if(d>0&&p>0){const f=document.createElement("canvas");f.width=d,f.height=p;const u=f.getContext("2d");u.translate(-n,-c),Cs(u,this._sourceCanvas,s,i,a),t.drawImage(f,n,c)}}else{const s=de(this._state);t.save(),t.transform(s.a,s.b,s.c,s.d,s.e,s.f),t.drawImage(this._sourceCanvas,0,0,this._state.width,this._state.height),t.restore()}}snapshot(){const t=this._getSnapshotBounds(),s=document.createElement("canvas");s.width=t.w,s.height=t.h;const i=s.getContext("2d");return i.save(),i.translate(-t.x,-t.y),this.renderTransformed(i),i.restore(),{canvas:s,...t}}commit(t){const s=t.getContext("2d");if(this._perspectiveActive){const i=[{x:0,y:0},{x:this._sourceCanvas.width,y:0},{x:this._sourceCanvas.width,y:this._sourceCanvas.height},{x:0,y:this._sourceCanvas.height}],a=ee(this._state,this._perspectiveCorners);Cs(s,this._sourceCanvas,i,a,32)}else{const i=de(this._state);s.save(),s.setTransform(i.a,i.b,i.c,i.d,i.e,i.f),s.drawImage(this._sourceCanvas,0,0,this._state.width,this._state.height),s.restore()}}cancel(){return this._sourceImageData}hasChanged(){const t=this._state,s=this._initialState;return t.x!==s.x||t.y!==s.y||t.width!==s.width||t.height!==s.height||t.rotation!==s.rotation||t.skewX!==s.skewX||t.skewY!==s.skewY||t.scaleX!==s.scaleX||t.scaleY!==s.scaleY||this._perspectiveActive}getState(){return this._state}getSourceRect(){return this._sourceRect}getBounds(){return this._getSnapshotBounds()}updateViewport(t,s){this._zoom=t,this._pan=s,this.renderPreview()}getCursor(t){const s=re(this._state,this._handleConfig,this._zoom);return Math.hypot(t.x-s.commitCenter.x,t.y-s.commitCenter.y)<=s.buttonRadius||Math.hypot(t.x-s.cancelCenter.x,t.y-s.cancelCenter.y)<=s.buttonRadius?"pointer":Ja(t,this._state,this._handleConfig,this._zoom)}_onChange(){this.renderPreview()}_getSnapshotBounds(){const t=this._perspectiveActive?ee(this._state,this._perspectiveCorners):[A({x:0,y:0},this._state),A({x:this._state.width,y:0},this._state),A({x:this._state.width,y:this._state.height},this._state),A({x:0,y:this._state.height},this._state)],s=t.map(c=>c.x),i=t.map(c=>c.y),a=Math.floor(Math.min(...s)),o=Math.floor(Math.min(...i)),r=Math.max(1,Math.ceil(Math.max(...s))-a),n=Math.max(1,Math.ceil(Math.max(...i))-o);return{x:a,y:o,w:r,h:n}}dispose(){}}var Qa=Object.getOwnPropertyDescriptor,to=(e,t,s,i)=>{for(var a=i>1?void 0:i?Qa(t,s):t,o=e.length-1,r;o>=0;o--)(r=e[o])&&(a=r(a)||a);return a};let Ee=class extends F{constructor(){super(...arguments),this._dialog=null,this._resolve=null,this._imgW=0,this._imgH=0,this._canvasW=0,this._canvasH=0}show(e,t,s,i){return new Promise(a=>{this._resolve=a,this._imgW=e,this._imgH=t,this._canvasW=s,this._canvasH=i,this.requestUpdate(),this.updateComplete.then(()=>{this._dialog=this.renderRoot.querySelector("dialog"),this._dialog?.showModal()})})}_onScale(){this._dialog?.close(),this._resolve?.(!0),this._resolve=null}_onKeep(){this._dialog?.close(),this._resolve?.(!1),this._resolve=null}render(){return g`
      <dialog @cancel=${e=>{e.preventDefault(),this._onKeep()}}>
        <p>
          This image (${this._imgW}&times;${this._imgH}) is larger than the canvas
          (${this._canvasW}&times;${this._canvasH}). Would you like to scale it to fit?
        </p>
        <div class="buttons">
          <button @click=${this._onKeep}>Keep original size</button>
          <button class="primary" @click=${this._onScale}>Scale to fit</button>
        </div>
      </dialog>
    `}};Ee.styles=yt`
    dialog {
      background: #2a2a2a;
      color: #e0e0e0;
      border: 1px solid #555;
      border-radius: 8px;
      padding: 24px;
      max-width: 400px;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 14px;
    }
    dialog::backdrop {
      background: rgba(0, 0, 0, 0.5);
    }
    p {
      margin: 0 0 16px;
      line-height: 1.5;
    }
    .buttons {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
    }
    button {
      padding: 8px 16px;
      border-radius: 4px;
      border: 1px solid #555;
      background: #3a3a3a;
      color: #e0e0e0;
      cursor: pointer;
      font-size: 13px;
    }
    button:hover {
      background: #4a4a4a;
    }
    button.primary {
      background: #4a90d9;
      border-color: #4a90d9;
    }
    button.primary:hover {
      background: #5aa0e9;
    }
  `;Ee=to([wt("resize-dialog")],Ee);var eo=Object.defineProperty,so=Object.getOwnPropertyDescriptor,Zt=(e,t,s,i)=>{for(var a=i>1?void 0:i?so(t,s):t,o=e.length-1,r;o>=0;o--)(r=e[o])&&(a=(i?r(t,s,a):r(a))||a);return i&&a&&eo(t,s,a),a};function ks(e){return e.before.width===1&&e.before.height===1&&!$e(e.before,e.after)}let T=class extends F{constructor(){super(...arguments),this._ctx=new gt(this,{context:Lt,subscribe:!0}),this._checkerboardPattern=null,this._resizeObserver=null,this._lastLayers=null,this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._panX=0,this._panY=0,this._panning=!1,this._panStartX=0,this._panStartY=0,this._panStartOffsetX=0,this._panStartOffsetY=0,this._panPointerId=-1,this._moveTempCanvas=null,this._moveStartPoint=null,this._zoom=1,this._pointers=new Map,this._pinching=!1,this._lastPinchDist=0,this._lastPinchMidX=0,this._lastPinchMidY=0,this._transformManager=null,this._transformContentMode="lifted",this._clipboard=null,this._clipboardOrigin=null,this._clipboardRotation=0,this._clipboardBlobSize=null,this._engine=new Ys,this._tintPreviewCanvas=null,this._strokeTintCanvas=null,this._samplingDirty=!0,this._compositeScheduler=Ne(()=>this.composite()),this._canvasRect=null,this._strokeTintNeedsClear=!0,this._samplingBuffer=null,this._altSampling=!1,this._lastPointerScreenX=0,this._lastPointerScreenY=0,this._pointerOnCanvas=!1,this._floatIsExternalImage=!1,this._selectionDrawing=!1,this._cropRectValue=null,this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._cropActionsVisible=!1,this._textEditing=!1,this._textPosition={x:0,y:0},this._textSelecting=!1,this._textSelectAnchor=0,this._textAreaEl=null,this._textCursorVisible=!1,this._textCursorInterval=0,this._history=[],this._historyIndex=-1,this._maxHistory=50,this._beforeDrawCanvas=null,this._beforeDrawBuffer=null,this._invalidateCanvasRect=()=>{this._canvasRect=null},this._onWheel=e=>{if(e.ctrlKey||e.metaKey){if(e.preventDefault(),e.deltaY===0)return;const t=this._getCanvasRect(),s=e.clientX-t.left,i=e.clientY-t.top,a=(s-this._panX)/this._zoom,o=(i-this._panY)/this._zoom,r=Math.max(-5,Math.min(5,-e.deltaY*.01)),n=Math.min(T.MAX_ZOOM,Math.max(T.MIN_ZOOM,this._zoom*(1+r)));if(n===this._zoom)return;this._panX=s-a*n,this._panY=i-o*n,this._zoom=n,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview(),this._dispatchZoomChange();return}e.preventDefault(),this._panX-=e.deltaX,this._panY-=e.deltaY,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview(),this._dispatchViewportChange()},this._onWindowBlur=()=>{this._altSampling=!1,this._clearEyedropperPreview()},this._onPointerEnter=e=>{this._pointerOnCanvas=!0;const t=this._getCanvasRect();this._lastPointerScreenX=e.clientX-t.left,this._lastPointerScreenY=e.clientY-t.top,this._renderPreview()},this._onApplyCropClick=()=>{this.commitCrop()},this._onCancelCropClick=()=>{this.cancelCrop()},this._onDragOver=e=>{e.preventDefault()},this._onDragEnter=e=>{e.preventDefault(),this.classList.add("drop-target")},this._onDragLeave=e=>{e.relatedTarget&&this.contains(e.relatedTarget)||this.classList.remove("drop-target")},this._onDrop=async e=>{if(e.preventDefault(),this.classList.remove("drop-target"),!!e.dataTransfer?.files.length)for(const t of Array.from(e.dataTransfer.files)){if(!t.type.startsWith("image/"))continue;const s=URL.createObjectURL(t),i=t.name.replace(/\.[^.]+$/,"")||"Dropped Image";try{const a=await new Promise((o,r)=>{const n=new Image;n.onload=()=>o(n),n.onerror=()=>r(new Error("Image load failed")),n.src=s});URL.revokeObjectURL(s),await this._handleExternalImage(a,i)}catch{URL.revokeObjectURL(s)}return}}}get ctx(){return this._ctx.value}get _cropRect(){return this._cropRectValue}set _cropRect(e){this._cropRectValue=e,this._updateCropActions()}_updateCropActions(){const e=this._cropRectValue;this._cropActionsVisible=e!==null&&!this._cropDragging&&this._cropHandle===null&&Math.abs(e.w)>=1&&Math.abs(e.h)>=1}get _docWidth(){return this._ctx.value?.state.documentWidth??800}get _docHeight(){return this._ctx.value?.state.documentHeight??600}getWidth(){return this._docWidth}getHeight(){return this._docHeight}invalidateSamplingBuffer(){this._samplingDirty=!0}scheduleComposite(){this._compositeScheduler.schedule()}isTransformActive(){return this._transformManager!==null}enterTransformMode(){if(this._transformManager)return;const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(r=>r.id===e.activeLayerId);if(!t)return;const s=t.canvas.getContext("2d"),i=s.getImageData(0,0,t.canvas.width,t.canvas.height),a=xs(i);if(!a)return;this._captureBeforeDraw();const o=s.getImageData(a.x,a.y,a.w,a.h);s.clearRect(0,0,t.canvas.width,t.canvas.height),this._transformContentMode="lifted",this._transformManager=new rt(o,a,this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}getTransformValues(){if(!this._transformManager)return null;const e=this._transformManager;return{x:e.x,y:e.y,width:e.width,height:e.height,rotation:e.rotation,skewX:e.skewX,skewY:e.skewY,flipH:e.flipH,flipV:e.flipV}}setTransformValue(e,t){if(!this._transformManager)return;const s=this._transformManager;switch(e){case"x":s.x=t;break;case"y":s.y=t;break;case"width":s.width=t;break;case"height":s.height=t;break;case"rotation":s.rotation=t;break;case"skewX":s.skewX=t;break;case"skewY":s.skewY=t;break;case"flipH":s.flipH=!s.flipH;break;case"flipV":s.flipV=!s.flipV;break}this.composite(),this.requestUpdate(),this._dispatchTransformChange()}commitTransform(){if(!this._transformManager)return;const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(o=>o.id===e.activeLayerId);if(!t)return;const s=t.canvas.getContext("2d"),i=this._transformContentMode==="inserted";let a=null;if(i){const o=this._transformManager.getBounds(),r=Math.max(0,o.x),n=Math.max(0,o.y),c=Math.min(this._docWidth,o.x+o.w),l=Math.min(this._docHeight,o.y+o.h),h=c-r,d=l-n;h>0&&d>0&&(a={x:r,y:n,w:h,h:d,before:s.getImageData(r,n,h,d)})}if(this._transformManager.commit(t.canvas),i&&a){const o=s.getImageData(a.x,a.y,a.w,a.h),r=$e(a.before,o);r&&this._pushHistoryEntry({type:"patch",layerId:t.id,x:a.x+r.x,y:a.y+r.y,before:Gt(a.before,r),after:Gt(o,r)})}else if(this._transformManager.hasChanged()&&this._beforeDrawCanvas){const o=this._readChangedPatch(s,void 0,!0);o&&this._pushHistoryEntry({type:"patch",layerId:t.id,...o})}this._beforeDrawCanvas=null,this._transformContentMode="lifted",this._floatIsExternalImage=!1,this._transformManager.dispose(),this._transformManager=null,this.previewCanvas.getContext("2d").clearRect(0,0,this.previewCanvas.width,this.previewCanvas.height),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}cancelTransform(){if(!this._transformManager)return;const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(i=>i.id===e.activeLayerId);if(!t)return;if(this._floatIsExternalImage){this.cancelExternalFloat();return}const s=t.canvas.getContext("2d");if(this._transformContentMode==="inserted")this._transformManager.cancel();else if(this._beforeDrawCanvas)this._restoreBeforeDraw(s);else{const i=this._transformManager.cancel(),a=this._transformManager.getSourceRect();s.putImageData(i,a.x,a.y)}this._transformManager.dispose(),this._transformManager=null,this._beforeDrawCanvas=null,this._transformContentMode="lifted",this.previewCanvas.getContext("2d").clearRect(0,0,this.previewCanvas.width,this.previewCanvas.height),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}get _vw(){return this.mainCanvas?.width??800}get _vh(){return this.mainCanvas?.height??600}get _brushDescriptor(){const e=this.ctx.state;if(e.brush)return e.brush;const t=ie();return{...t,size:e.brushSize??t.size,tip:{...t.tip},ink:{...t.ink}}}_getActiveLayerCtx(){const e=this._ctx.value?.state;return e?e.layers.find(s=>s.id===e.activeLayerId)?.canvas.getContext("2d")??null:null}composite(){if(!this.mainCanvas)return;this._compositeScheduler.cancel();const e=this.mainCanvas.getContext("2d"),t=this._vw,s=this._vh;e.fillStyle="#3a3a3a",e.fillRect(0,0,t,s),e.save(),e.translate(this._panX,this._panY),e.scale(this._zoom,this._zoom),e.save(),e.beginPath(),e.rect(0,0,this._docWidth,this._docHeight),e.clip();const i=this._getCheckerboardPattern(e);e.fillStyle=i,e.fillRect(0,0,this._docWidth,this._docHeight),e.restore();const a=this._ctx.value?.state.layers??[],o=this._ctx.value?.state.activeLayerId??null,r=a.some(n=>n.visible&&n.blendMode!=="normal");for(const n of a){if(!n.visible)continue;e.globalAlpha=n.opacity,r&&(e.globalCompositeOperation=Yt(n.blendMode));const c=this._drawing&&n.id===o?this._engine.getStrokePreview():null,l=c?.bounds??null;if(c&&l)if(!c.eraser&&n.opacity>=1&&n.blendMode==="normal"){e.drawImage(n.canvas,0,0);const d=c.color===null?c.canvas:this._tintStrokeRegion(c.canvas,l,c.color);e.globalAlpha=c.opacity,e.drawImage(d,l.x,l.y,l.w,l.h,l.x,l.y,l.w,l.h),e.globalAlpha=n.opacity}else{(!this._tintPreviewCanvas||this._tintPreviewCanvas.width!==this._docWidth||this._tintPreviewCanvas.height!==this._docHeight)&&(this._tintPreviewCanvas=document.createElement("canvas"),this._tintPreviewCanvas.width=this._docWidth,this._tintPreviewCanvas.height=this._docHeight);const d=this._tintPreviewCanvas.getContext("2d");if(d.globalCompositeOperation="source-over",d.clearRect(0,0,this._docWidth,this._docHeight),d.drawImage(n.canvas,0,0),d.globalAlpha=c.opacity,c.eraser)d.globalCompositeOperation="destination-out",d.drawImage(c.canvas,l.x,l.y,l.w,l.h,l.x,l.y,l.w,l.h);else if(c.color===null)d.drawImage(c.canvas,l.x,l.y,l.w,l.h,l.x,l.y,l.w,l.h);else{const p=this._tintStrokeRegion(c.canvas,l,c.color);d.drawImage(p,l.x,l.y,l.w,l.h,l.x,l.y,l.w,l.h)}d.globalAlpha=1,d.globalCompositeOperation="source-over",e.drawImage(this._tintPreviewCanvas,0,0)}else e.drawImage(n.canvas,0,0);this._transformManager&&n.id===o&&this._transformManager.renderTransformed(e),r&&(e.globalCompositeOperation="source-over"),e.globalAlpha=1}e.strokeStyle="rgba(0,0,0,0.3)",e.lineWidth=1,e.strokeRect(-.5,-.5,this._docWidth+1,this._docHeight+1),e.restore(),this.dispatchEvent(new CustomEvent("composited",{bubbles:!0,composed:!0,detail:null})),this.invalidateSamplingBuffer()}_tintStrokeRegion(e,t,s){const i=this._docWidth,a=this._docHeight;(!this._strokeTintCanvas||this._strokeTintCanvas.width!==i||this._strokeTintCanvas.height!==a)&&(this._strokeTintCanvas=document.createElement("canvas"),this._strokeTintCanvas.width=i,this._strokeTintCanvas.height=a,this._strokeTintNeedsClear=!1);const o=this._strokeTintCanvas.getContext("2d");return this._strokeTintNeedsClear&&(o.clearRect(0,0,i,a),this._strokeTintNeedsClear=!1),o.save(),o.beginPath(),o.rect(t.x,t.y,t.w,t.h),o.clip(),o.globalCompositeOperation="source-over",o.clearRect(t.x,t.y,t.w,t.h),o.drawImage(e,t.x,t.y,t.w,t.h,t.x,t.y,t.w,t.h),Ue(o,s,i,a),o.restore(),this._strokeTintCanvas}_getCheckerboardPattern(e){if(!this._checkerboardPattern){const t=document.createElement("canvas");t.width=20,t.height=20;const s=t.getContext("2d");s.fillStyle="#ffffff",s.fillRect(0,0,20,20),s.fillStyle="#e0e0e0",s.fillRect(10,0,10,10),s.fillRect(0,10,10,10),this._checkerboardPattern=e.createPattern(t,"repeat")}return this._checkerboardPattern}willUpdate(){const e=this._ctx.value?.state.layers??null;if(e&&e!==this._lastLayers&&(this._lastLayers=e,this.mainCanvas&&this.composite()),this.mainCanvas&&this._ctx.value){const t=this._ctx.value.state.activeTool;t==="hand"?this.mainCanvas.style.cursor=this._panning?"grabbing":"grab":t==="move"?this.mainCanvas.style.cursor="move":t==="text"?this.mainCanvas.style.cursor="text":this.mainCanvas.style.cursor="crosshair",this._textEditing&&this._renderTextPreview()}this._renderPreview()}firstUpdated(){const e=this.getBoundingClientRect(),t=e.width>0?Math.floor(e.width):800,s=e.height>0?Math.floor(e.height):600;this.mainCanvas.width=t,this.mainCanvas.height=s,this.previewCanvas.width=t,this.previewCanvas.height=s,this._panX=Math.round((t-this._docWidth*this._zoom)/2),this._panY=Math.round((s-this._docHeight*this._zoom)/2),this._resizeObserver=new ResizeObserver(()=>{this._invalidateCanvasRect(),this._resizeToFit()}),this._resizeObserver.observe(this);const i=this._getActiveLayerCtx();i&&(i.fillStyle="#ffffff",i.fillRect(0,0,this._docWidth,this._docHeight)),this.composite(),this._dispatchViewportChange(),this._textAreaEl&&this.shadowRoot.appendChild(this._textAreaEl)}centerDocument(){this.mainCanvas&&(this._panX=Math.round((this._vw-this._docWidth*this._zoom)/2),this._panY=Math.round((this._vh-this._docHeight*this._zoom)/2),this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._dispatchViewportChange())}_resizeToFit(){const e=this.getBoundingClientRect();if(e.width<=0||e.height<=0)return;const t=Math.floor(e.width),s=Math.floor(e.height),i=this.mainCanvas.width,a=this.mainCanvas.height;if(i===t&&a===s)return;this.mainCanvas.width=t,this.mainCanvas.height=s,this.previewCanvas.width=t,this.previewCanvas.height=s;const o=(i/2-this._panX)/this._zoom,r=(a/2-this._panY)/this._zoom;this._panX=t/2-o*this._zoom,this._panY=s/2-r*this._zoom,this._checkerboardPattern=null,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._dispatchViewportChange()}getHistory(){return[...this._history]}getHistoryIndex(){return this._historyIndex}setHistory(e,t){this._history=e,this._historyIndex=Math.max(-1,Math.min(t,e.length-1)),this._notifyHistory()}_captureBeforeDraw(){const e=this._getActiveLayerCtx();if(!e)return;const{width:t,height:s}=e.canvas;let i=this._beforeDrawBuffer;(!i||i.width!==t||i.height!==s)&&(i=document.createElement("canvas"),i.width=t,i.height=s,this._beforeDrawBuffer=i);const a=i.getContext("2d");a.clearRect(0,0,t,s),a.drawImage(e.canvas,0,0),this._beforeDrawCanvas=i}_restoreBeforeDraw(e){const t=this._beforeDrawCanvas;t&&(e.save(),e.setTransform(1,0,0,1,0,0),e.globalAlpha=1,e.globalCompositeOperation="source-over",e.clearRect(0,0,e.canvas.width,e.canvas.height),e.drawImage(t,0,0),e.restore())}_pushDrawHistory(e=!1,t){const s=this._ctx.value?.state,i=this._getActiveLayerCtx();if(!i||!s||!this._beforeDrawCanvas)return;const a=this._readChangedPatch(i,t,e);this._beforeDrawCanvas=null,a&&this._pushHistoryEntry({type:"patch",layerId:s.activeLayerId,...a})}_readChangedPatch(e,t,s){const i=this._beforeDrawCanvas,a=Math.min(i.width,e.canvas.width),o=Math.min(i.height,e.canvas.height),r=t??{x:0,y:0,w:a,h:o},n=Math.max(0,Math.floor(r.x)),c=Math.max(0,Math.floor(r.y)),l=Math.min(a,Math.ceil(r.x+r.w))-n,h=Math.min(o,Math.ceil(r.y+r.h))-c,d=i.getContext("2d");let p=null,f=null,u=null;return l>0&&h>0&&(f=d.getImageData(n,c,l,h),u=e.getImageData(n,c,l,h),p=$e(f,u)),p&&f&&u?{x:n+p.x,y:c+p.y,before:Gt(f,p),after:Gt(u,p)}:!s||a<=0||o<=0?null:{x:0,y:0,before:d.getImageData(0,0,1,1),after:e.getImageData(0,0,1,1)}}_commitStroke(e){if(!this._engine.commit(e))return;const t=this._engine.getDirtyBounds();if(!t)return{x:0,y:0,w:0,h:0};const s=2;return{x:t.x-s,y:t.y-s,w:t.w+s*2,h:t.h+s*2}}pushLayerOperation(e){this._pushHistoryEntry(e)}_pushHistoryEntry(e){this._history=this._history.slice(0,this._historyIndex+1),this._history.push(e),this._history.length>this._maxHistory?this._history.shift():this._historyIndex++,this._notifyHistory()}_getEntryLayerId(e){switch(e.type){case"draw":case"patch":case"visibility":case"opacity":case"rename":case"blend-mode":case"transform":return e.layerId;case"add-layer":case"delete-layer":return e.layer.id;case"reorder":return null;case"crop":case"merge":return null}}_notifyHistory(){this.dispatchEvent(new CustomEvent("history-change",{bubbles:!0,composed:!0,detail:{canUndo:this._historyIndex>=0||this._transformManager!==null,canRedo:this._historyIndex<this._history.length-1}}))}_dispatchTransformChange(){this.dispatchEvent(new CustomEvent("transform-change",{bubbles:!0,composed:!0,detail:{active:this._transformManager!==null,values:this.getTransformValues()}}))}_transformValuesEqual(e,t){return e===t?!0:!e||!t?!1:e.x===t.x&&e.y===t.y&&e.width===t.width&&e.height===t.height&&e.rotation===t.rotation&&e.skewX===t.skewX&&e.skewY===t.skewY&&e.flipH===t.flipH&&e.flipV===t.flipV}undo(){if(this._textEditing&&this._commitText(),this._drawing){const t=this._getActiveLayerCtx(),s=t?this._commitStroke(t):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null;const i=this._beforeDrawCanvas!==null;if(this._pushDrawHistory(!1,s),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),!i)return}if(this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._transformManager){this.cancelTransform();return}if(this._historyIndex<0)return;const e=this._history[this._historyIndex];this._historyIndex--,this._applyUndo(e),this.composite(),this._notifyHistory()}redo(){if(this._textEditing&&this._commitText(),this._drawing){const t=this._getActiveLayerCtx(),s=t?this._commitStroke(t):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null;const i=this._beforeDrawCanvas!==null;if(this._pushDrawHistory(!1,s),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),!i)return}if(this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._historyIndex>=this._history.length-1)return;this._transformManager&&this.cancelTransform(),this._historyIndex++;const e=this._history[this._historyIndex];this._applyRedo(e),this.composite(),this._notifyHistory()}_applyUndo(e){const t=this._ctx.value?.state;if(t)switch(e.type){case"draw":case"transform":{const s=t.layers.find(i=>i.id===e.layerId);s&&s.canvas.getContext("2d").putImageData(e.before,0,0);break}case"patch":{const s=t.layers.find(i=>i.id===e.layerId);s&&!ks(e)&&s.canvas.getContext("2d").putImageData(e.before,e.x,e.y);break}case"add-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:e.layer.id}}));break}case"delete-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"restore-layer",snapshot:e.layer,index:e.index}}));break}case"reorder":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"reorder",fromIndex:e.toIndex,toIndex:e.fromIndex}}));break}case"visibility":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.visible=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"opacity":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.opacity=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"rename":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.name=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"blend-mode":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.blendMode=e.before,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"crop":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"crop-restore",layers:e.beforeLayers,width:e.beforeWidth,height:e.beforeHeight}}));break}case"merge":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"stack-replace",layers:e.beforeLayers,activeLayerId:e.previousActiveLayerId}}));break}}}_applyRedo(e){const t=this._ctx.value?.state;if(t)switch(e.type){case"draw":case"transform":{const s=t.layers.find(i=>i.id===e.layerId);s&&s.canvas.getContext("2d").putImageData(e.after,0,0);break}case"patch":{const s=t.layers.find(i=>i.id===e.layerId);s&&!ks(e)&&s.canvas.getContext("2d").putImageData(e.after,e.x,e.y);break}case"add-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"restore-layer",snapshot:e.layer,index:e.index}}));break}case"delete-layer":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:e.layer.id}}));break}case"reorder":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"reorder",fromIndex:e.fromIndex,toIndex:e.toIndex}}));break}case"visibility":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.visible=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"opacity":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.opacity=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"rename":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.name=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"blend-mode":{const s=t.layers.find(i=>i.id===e.layerId);s&&(s.blendMode=e.after,this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"refresh"}})));break}case"crop":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"crop-restore",layers:e.afterLayers,width:e.afterWidth,height:e.afterHeight}}));break}case"merge":{this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"stack-replace",layers:e.afterLayers,activeLayerId:e.afterActiveLayerId}}));break}}}clearCanvas(){if(this._drawing){const t=this._getActiveLayerCtx(),s=t?this._commitStroke(t):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!0,s)}this.clearSelection(),this._captureBeforeDraw();const e=this._getActiveLayerCtx();e&&e.clearRect(0,0,this._docWidth,this._docHeight),this._pushDrawHistory(!0),this.composite()}renderFlattened(e="#ffffff"){const t=document.createElement("canvas");t.width=this._docWidth,t.height=this._docHeight;const s=t.getContext("2d");e&&(s.fillStyle=e,s.fillRect(0,0,this._docWidth,this._docHeight));const i=this._ctx.value?.state,a=i?.layers??[],o=i?.activeLayerId??null;for(const r of a)r.visible&&(s.globalAlpha=r.opacity,s.globalCompositeOperation=Yt(r.blendMode),s.drawImage(r.canvas,0,0),this._transformManager&&r.id===o&&this._transformManager.renderTransformed(s),s.globalCompositeOperation="source-over",s.globalAlpha=1);return t}saveCanvas(){const e=this.renderFlattened("#ffffff"),t=document.createElement("a");t.download="drawing.png",t.href=e.toDataURL("image/png"),t.click()}_getCanvasRect(){return this._canvasRect||(this._canvasRect=this.mainCanvas.getBoundingClientRect()),this._canvasRect}_getDocPoint(e){const t=this._getCanvasRect();return{x:(e.clientX-t.left-this._panX)/this._zoom,y:(e.clientY-t.top-this._panY)/this._zoom}}_clientToDoc(e,t){const s=this._getCanvasRect();return{x:(e-s.left-this._panX)/this._zoom,y:(t-s.top-this._panY)/this._zoom}}_startPan(e){this._panning=!0,this._panStartX=e.clientX,this._panStartY=e.clientY,this._panStartOffsetX=this._panX,this._panStartOffsetY=this._panY,this._panPointerId=e.pointerId,this.mainCanvas.setPointerCapture(e.pointerId),this.mainCanvas.style.cursor="grabbing"}_updatePan(e){this._panning&&(this._panX=this._panStartOffsetX+(e.clientX-this._panStartX),this._panY=this._panStartOffsetY+(e.clientY-this._panStartY),this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview())}_endPan(){if(!this._panning)return;const e=this._panPointerId;if(this._panning=!1,this._panPointerId=-1,e>=0&&this.mainCanvas)try{this.mainCanvas.releasePointerCapture(e)}catch{}if(this._ctx.value){const t=this._ctx.value.state.activeTool;t==="hand"?this.mainCanvas.style.cursor="grab":t==="move"?this.mainCanvas.style.cursor="move":this.mainCanvas.style.cursor="crosshair"}this._dispatchViewportChange()}_dispatchZoomChange(){this.dispatchEvent(new CustomEvent("zoom-change",{bubbles:!0,composed:!0,detail:{zoom:this._zoom}})),this._dispatchViewportChange()}_dispatchViewportChange(){this.dispatchEvent(new CustomEvent("viewport-change",{bubbles:!0,composed:!0}))}zoomIn(){this._zoomToCenter(this._zoom*T.ZOOM_STEP)}zoomOut(){this._zoomToCenter(this._zoom/T.ZOOM_STEP)}zoomToFit(){const e=Math.min(this._vw/this._docWidth,this._vh/this._docHeight)*.9;this._zoom=Math.min(T.MAX_ZOOM,Math.max(T.MIN_ZOOM,e)),this._panX=Math.round((this._vw-this._docWidth*this._zoom)/2),this._panY=Math.round((this._vh-this._docHeight*this._zoom)/2),this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._dispatchZoomChange()}getZoom(){return this._zoom}getViewport(){return{zoom:this._zoom,panX:this._panX,panY:this._panY}}setViewport(e,t,s){this._zoom=Math.min(T.MAX_ZOOM,Math.max(T.MIN_ZOOM,e)),this._panX=t,this._panY=s,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview(),this._dispatchViewportChange()}_zoomToCenter(e){const t=Math.min(T.MAX_ZOOM,Math.max(T.MIN_ZOOM,e));if(t===this._zoom)return;const s=this._vw/2,i=this._vh/2,a=(s-this._panX)/this._zoom,o=(i-this._panY)/this._zoom;this._panX=s-a*t,this._panY=i-o*t,this._zoom=t,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.composite(),this._textEditing&&this._renderTextPreview(),this._dispatchZoomChange()}_ensureSamplingBuffer(){(!this._samplingBuffer||this._samplingBuffer.width!==this._docWidth||this._samplingBuffer.height!==this._docHeight)&&(this._samplingBuffer=document.createElement("canvas"),this._samplingBuffer.width=this._docWidth,this._samplingBuffer.height=this._docHeight,this._samplingDirty=!0);const e=this._samplingBuffer.getContext("2d",{willReadFrequently:!0});if(this._samplingDirty){e.clearRect(0,0,this._docWidth,this._docHeight),e.fillStyle="#ffffff",e.fillRect(0,0,this._docWidth,this._docHeight);const t=this._ctx.value?.state.layers??[],s=this._ctx.value?.state.activeLayerId??null;for(const i of t)i.visible&&(e.globalAlpha=i.opacity,e.globalCompositeOperation=Yt(i.blendMode),e.drawImage(i.canvas,0,0),e.globalCompositeOperation="source-over",this._transformManager&&i.id===s&&this._transformManager.renderTransformed(e));e.globalAlpha=1,this._samplingDirty=!1}return e}_sampleColor(e,t){const s=Math.round(e),i=Math.round(t);if(s<0||i<0||s>=this._docWidth||i>=this._docHeight)return null;if(this.ctx.state.eyedropperSampleAll){const r=this._ensureSamplingBuffer().getImageData(s,i,1,1).data;return`#${r[0].toString(16).padStart(2,"0")}${r[1].toString(16).padStart(2,"0")}${r[2].toString(16).padStart(2,"0")}`}else{const o=this._getActiveLayerCtx();if(!o)return null;const r=o.getImageData(s,i,1,1).data;if(r[3]===0)return null;if(r[3]<255){const n=r[3]/255,c=Math.round(r[0]*n+255*(1-n)),l=Math.round(r[1]*n+255*(1-n)),h=Math.round(r[2]*n+255*(1-n));return`#${c.toString(16).padStart(2,"0")}${l.toString(16).padStart(2,"0")}${h.toString(16).padStart(2,"0")}`}return`#${r[0].toString(16).padStart(2,"0")}${r[1].toString(16).padStart(2,"0")}${r[2].toString(16).padStart(2,"0")}`}}_renderEyedropperPreview(e){const t=this.previewCanvas.getContext("2d");t.clearRect(0,0,this._vw,this._vh);const s=this._getDocPoint(e),i=this._sampleColor(s.x,s.y),a=this.ctx.state.eyedropperSampleAll,o=a?this.mainCanvas:this._getActiveLayerCtx()?.canvas??this.mainCanvas,r=88,n=24,c=r+n+4,l=20,h=this._getCanvasRect();let d=e.clientX-h.left+l,p=e.clientY-h.top-l-c;if(d+r>this._vw&&(d=d-r-2*l),p<0&&(p=p+c+2*l),t.save(),t.imageSmoothingEnabled=!1,a){const m=e.clientX-h.left,v=e.clientY-h.top;t.drawImage(this.mainCanvas,m-5,v-5,11,11,d,p,r,r)}else{const m=Math.round(s.x),v=Math.round(s.y);t.drawImage(o,m-5,v-5,11,11,d,p,r,r)}t.restore(),t.strokeStyle="rgba(255,255,255,0.3)",t.lineWidth=.5;const f=r/11;for(let m=0;m<=11;m++){const v=d+m*f,b=p+m*f;t.beginPath(),t.moveTo(v,p),t.lineTo(v,p+r),t.stroke(),t.beginPath(),t.moveTo(d,b),t.lineTo(d+r,b),t.stroke()}const u=d+5*f,_=p+5*f;t.strokeStyle="#fff",t.lineWidth=1.5,t.strokeRect(u,_,f,f),t.strokeStyle="#555",t.lineWidth=1,t.strokeRect(d-.5,p-.5,r+1,c+1),i&&(t.fillStyle=i,t.fillRect(d,p+r+2,n,n),t.fillStyle="#fff",t.font="11px monospace",t.fillText(i.toUpperCase(),d+n+6,p+r+16))}_clearEyedropperPreview(){const e=this.previewCanvas?.getContext("2d");e&&e.clearRect(0,0,this._vw,this._vh)}_renderBrushCursor(){if(!this._pointerOnCanvas||this._altSampling)return;const{activeTool:e}=this.ctx.state,t=this._brushDescriptor,s=t.size,i=t.hardness;if(e!=="pencil"&&e!=="eraser")return;const a=this.previewCanvas.getContext("2d"),o=this._lastPointerScreenX,r=this._lastPointerScreenY,n=s/2*this._zoom;if(a.beginPath(),a.arc(o,r,n,0,Math.PI*2),a.strokeStyle="rgba(0,0,0,0.7)",a.lineWidth=1.5,a.stroke(),a.beginPath(),a.arc(o,r,n,0,Math.PI*2),a.strokeStyle="rgba(255,255,255,0.7)",a.lineWidth=.75,a.stroke(),i<1){const c=n*i;a.beginPath(),a.arc(o,r,c,0,Math.PI*2),a.setLineDash([3,3]),a.strokeStyle="rgba(255,255,255,0.5)",a.lineWidth=.75,a.stroke(),a.setLineDash([])}}_renderStampCursor(){if(!this._pointerOnCanvas||this._transformManager)return;const e=this.ctx.state.stampImage;if(!e||e.naturalWidth<=0||e.naturalHeight<=0)return;const t=this.previewCanvas.getContext("2d"),i=this.ctx.state.stampSize/Math.max(e.naturalWidth,e.naturalHeight),a=Math.max(1,e.naturalWidth*i)*this._zoom,o=Math.max(1,e.naturalHeight*i)*this._zoom,r=this._lastPointerScreenX-a/2,n=this._lastPointerScreenY-o/2;t.save(),t.globalAlpha=.55,t.drawImage(e,r,n,a,o),t.globalAlpha=1,t.strokeStyle="rgba(255,255,255,0.9)",t.lineWidth=1,t.setLineDash([4,3]),t.strokeRect(r-.5,n-.5,a+1,o+1),t.restore()}_renderPreview(){const e=this.previewCanvas?.getContext("2d");if(!e)return;if(this._transformManager){this._transformManager.renderPreview();return}const{activeTool:t}=this.ctx.state;if(t==="pencil"||t==="eraser"||t==="eyedropper"||t==="stamp"){if(e.clearRect(0,0,this._vw,this._vh),this._altSampling||t==="eyedropper")return;t==="stamp"?this._renderStampCursor():this._drawing||this._renderBrushCursor()}}_onPointerDown(e){if(this._invalidateCanvasRect(),this._pointers.set(e.pointerId,{x:e.clientX,y:e.clientY}),this._pointers.size===2){this._enterPinchMode(e);return}if(this._pointers.size>2||!this._ctx.value)return;if(e.button===1){e.preventDefault(),this._startPan(e);return}if(e.button!==0)return;if(this._transformManager){const a=this._getDocPoint(e);e.pointerType==="touch"&&this._transformManager.setTouchMode(!0);const o={shift:e.shiftKey,ctrl:e.ctrlKey||e.metaKey,alt:e.altKey};this._transformManager.onPointerDown(a,o),this.mainCanvas.setPointerCapture(e.pointerId);return}const{activeTool:t}=this.ctx.state;if(e.altKey&&(t==="pencil"||t==="eraser")){this._altSampling=!0;const a=this._getDocPoint(e),o=this._sampleColor(a.x,a.y);o&&this.ctx.setStrokeColor(o);return}if(t==="hand"){this._startPan(e);return}if(t==="crop"){this.mainCanvas.setPointerCapture(e.pointerId);const a=this._getDocPoint(e);this._handleCropPointerDown(a);return}if(t==="eyedropper"){const a=this._getDocPoint(e),o=this._sampleColor(a.x,a.y);o&&this.ctx.setStrokeColor(o);return}const s=this.ctx.state.layers.find(a=>a.id===this.ctx.state.activeLayerId);if(s&&!s.visible)return;if(t==="move"){this.mainCanvas.setPointerCapture(e.pointerId),this._transformManager&&this.commitTransform();const a=this._getDocPoint(e);this._captureBeforeDraw();const o=this._getActiveLayerCtx();if(!o)return;const r=document.createElement("canvas");r.width=this._docWidth,r.height=this._docHeight,r.getContext("2d").drawImage(o.canvas,0,0),this._moveTempCanvas=r,this._moveStartPoint=a;return}this.mainCanvas.setPointerCapture(e.pointerId);const i=this._getDocPoint(e);if(t==="select"){this._handleSelectPointerDown(i);return}if(t==="fill"){const a=Math.round(i.x),o=Math.round(i.y);if(a>=0&&o>=0&&a<this._docWidth&&o<this._docHeight){const r=this._getActiveLayerCtx();r&&(this._captureBeforeDraw(),ja(r,a,o,this.ctx.state.strokeColor)?(this._pushDrawHistory(),this.composite()):this._beforeDrawCanvas=null)}return}if(t==="stamp"){this._transformManager&&this.commitTransform(),this.ctx.state.stampImage&&(this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this._createStampAsTransform(this.ctx.state.stampImage,i.x,i.y,this.ctx.state.stampSize,e.pointerType==="touch"));return}if(t==="text"){if(e.preventDefault(),this._textEditing){const a=this._getTextBoundingBox();if(i.x>=a.x&&i.x<=a.x+a.w&&i.y>=a.y&&i.y<=a.y+a.h){const o=this._pointToTextOffset(i);this._textAreaEl&&(this._textAreaEl.selectionStart=o,this._textAreaEl.selectionEnd=o),this._textSelectAnchor=o,this._textSelecting=!0,this._startTextCursorBlink(),this._renderTextPreview();return}this._commitText();return}this._textPosition=i,this._textEditing=!0,this._textAreaEl&&(this._textAreaEl.value="",this._textAreaEl.focus()),this._startTextCursorBlink(),this._renderTextPreview();return}if(this._drawing=!0,this._lastPoint=i,this._startPoint=i,t==="pencil"||t==="eraser"){this.previewCanvas?.getContext("2d")?.clearRect(0,0,this._vw,this._vh),this._captureBeforeDraw();const a=this._brushDescriptor,o=this.ctx.state.strokeColor,r=this.ctx.state.activeTool==="eraser";this._engine.begin(a,o,r,this._docWidth,this._docHeight),this._strokeTintNeedsClear=!0;const n=a.ink.wetness>0?this._getActiveLayerCtx()??void 0:void 0;this._engine.stroke(i.x,i.y,_s(e),n,e.timeStamp),this.composite()}}_onPointerMove(e){this._pointers.has(e.pointerId)&&this._pointers.set(e.pointerId,{x:e.clientX,y:e.clientY});const t=this._getCanvasRect();if(this._lastPointerScreenX=e.clientX-t.left,this._lastPointerScreenY=e.clientY-t.top,this._pinching){this._updatePinch();return}if(!this._ctx.value)return;if(this._transformManager){const a=this._getDocPoint(e),o={shift:e.shiftKey,ctrl:e.ctrlKey||e.metaKey,alt:e.altKey},r=this.getTransformValues();this._transformManager.onPointerMove(a,o),this.style.cursor=this._transformManager.getCursor(a),this.scheduleComposite();const n=this.getTransformValues();this._transformValuesEqual(r,n)||this._dispatchTransformChange();return}{const a=this.ctx.state.activeTool;if(a==="eyedropper"){this._renderEyedropperPreview(e);return}if(this._altSampling||e.altKey&&(a==="pencil"||a==="eraser")){if(this._altSampling=e.altKey,!e.altKey){this._clearEyedropperPreview();return}this._renderEyedropperPreview(e);return}}if(this._panning){this._updatePan(e);return}if(this._textSelecting){const a=this._getDocPoint(e),o=this._pointToTextOffset(a);if(this._textAreaEl){const r=this._textSelectAnchor;this._textAreaEl.selectionStart=Math.min(r,o),this._textAreaEl.selectionEnd=Math.max(r,o)}this._renderTextPreview();return}const{activeTool:s}=this.ctx.state;if(s==="crop"){this._handleCropPointerMove(e);return}if(s==="move"&&this._moveTempCanvas&&this._moveStartPoint){const a=this._getDocPoint(e);let o=a.x-this._moveStartPoint.x,r=a.y-this._moveStartPoint.y;e.shiftKey&&(Math.abs(o)>Math.abs(r)?r=0:o=0);const n=this._getActiveLayerCtx();n&&(n.clearRect(0,0,this._docWidth,this._docHeight),n.drawImage(this._moveTempCanvas,Math.round(o),Math.round(r)),this.scheduleComposite());return}if(s==="select"){this._handleSelectPointerMove(e);return}if(s==="stamp"){this._renderPreview();return}if(!this._drawing&&(s==="pencil"||s==="eraser")&&this._renderPreview(),!this._drawing||!this._lastPoint)return;const i=this._getDocPoint(e);if(s==="pencil"||s==="eraser"){const o=this._brushDescriptor.ink.wetness>0?this._getActiveLayerCtx()??void 0:void 0;this._engine.stroke(i.x,i.y,_s(e),o,e.timeStamp),this._lastPoint=i,this.scheduleComposite()}else if(vt(s)){const a=this.previewCanvas.getContext("2d");a.clearRect(0,0,this._vw,this._vh),a.save(),a.translate(this._panX,this._panY),a.scale(this._zoom,this._zoom),ns(a,s,this._startPoint,i,this.ctx.state.strokeColor,this.ctx.state.fillColor,this.ctx.state.useFill,this._brushDescriptor.size),a.restore()}}_onPointerUp(e){if(this._pointers.delete(e.pointerId),this._pinching){this._pointers.size<2&&(this._pinching=!1);return}if(!this._ctx.value)return;if(this._transformManager){const a=this._getDocPoint(e),o=this._transformManager.onPointerUp(a);o==="commit"||o==="commit-button"?this.commitTransform():o==="cancel-button"&&this.cancelTransform(),this.composite();return}if(this._panning){this._endPan();return}if(this._textSelecting){this._textSelecting=!1;return}const{activeTool:t}=this.ctx.state;if(t==="crop"){this._handleCropPointerUp();return}if(this._drawing&&t!=="pencil"&&t!=="eraser"&&!vt(t)){const a=this._getActiveLayerCtx(),o=a?this._commitStroke(a):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!0,o),this.composite();return}if(this._moveTempCanvas&&t!=="move"){this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(!0),this.composite();return}if(t==="move"&&this._moveTempCanvas){this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite();return}if(t==="select"){this._handleSelectPointerUp(e);return}if(!this._drawing)return;const s=this._getDocPoint(e);if(vt(t)){this._captureBeforeDraw();const a=this._getActiveLayerCtx();a&&ns(a,t,this._startPoint,s,this.ctx.state.strokeColor,this.ctx.state.fillColor,this.ctx.state.useFill,this._brushDescriptor.size),this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}let i;if(t==="pencil"||t==="eraser"){const a=this._getActiveLayerCtx();a&&(i=this._commitStroke(a))}this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!1,i),this.composite()}_onPointerLeave(e){if(this._pointerOnCanvas=!1,this._renderPreview(),!this._pinching){try{if(this.mainCanvas.hasPointerCapture(e.pointerId))return}catch{}this._onPointerUp(e)}}_onPointerCancel(e){this._pointers.delete(e.pointerId),this._pinching?this._pointers.size<2&&(this._pinching=!1):this._cancelCurrentTool(e.pointerId)}_cancelCurrentTool(e){try{this.mainCanvas.releasePointerCapture(e)}catch{}if(this._engine.cancel(),this._drawing){if(this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._beforeDrawCanvas){const s=this._getActiveLayerCtx();s&&this._restoreBeforeDraw(s),this._beforeDrawCanvas=null}this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite()}if(this._panning&&this._endPan(),this._moveTempCanvas){if(this._beforeDrawCanvas){const s=this._getActiveLayerCtx();s&&this._restoreBeforeDraw(s),this._beforeDrawCanvas=null}this._moveTempCanvas=null,this._moveStartPoint=null,this.composite()}this._selectionDrawing&&(this._selectionDrawing=!1,this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh));const t=this._cropDragging||this._cropHandle!==null;if(this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,t&&this._cropRect){const s=Se(this._ctx.value?.state.cropAspectRatio??"free"),i=this._normalizeCropRect(this._cropRect,s);this._cropRect=i.w<1||i.h<1?null:i,this._cropRect?this._drawCropPreview():this._clearCropPreview()}this._updateCropActions()}_enterPinchMode(e){for(const[a]of this._pointers)if(a!==e.pointerId){this._cancelCurrentTool(a);break}this._pinching=!0;const t=[...this._pointers.values()],s=t[1].x-t[0].x,i=t[1].y-t[0].y;this._lastPinchDist=Math.hypot(s,i),this._lastPinchMidX=(t[0].x+t[1].x)/2,this._lastPinchMidY=(t[0].y+t[1].y)/2}_updatePinch(){const e=[...this._pointers.values()];if(e.length<2)return;const t=e[1].x-e[0].x,s=e[1].y-e[0].y,i=Math.hypot(t,s),a=(e[0].x+e[1].x)/2,o=(e[0].y+e[1].y)/2,r=a-this._lastPinchMidX,n=o-this._lastPinchMidY;if(this._panX+=r,this._panY+=n,this._lastPinchDist>0){const c=i/this._lastPinchDist,l=this._getCanvasRect(),h=a-l.left,d=o-l.top,p=(h-this._panX)/this._zoom,f=(d-this._panY)/this._zoom,u=Math.min(T.MAX_ZOOM,Math.max(T.MIN_ZOOM,this._zoom*c));this._panX=h-p*u,this._panY=d-f*u,this._zoom=u}this._lastPinchDist=i,this._lastPinchMidX=a,this._lastPinchMidY=o,this._transformManager?.updateViewport(this._zoom,{x:this._panX,y:this._panY}),this.scheduleComposite(),this._textEditing&&this._renderTextPreview(),this._dispatchZoomChange()}_handleSelectPointerDown(e){this._transformManager&&this.commitTransform(),this._selectionDrawing=!0,this._startPoint=e}_handleCropPointerDown(e){if(this._cropRect){const t=gs(this._cropRect,e,this._zoom);if(t&&t!=="move"){this._cropHandle=t,this._cropDragOrigin={x:e.x,y:e.y},this._cropRectOrigin={...this._cropRect},this._updateCropActions();return}if(t==="move"){this._cropHandle="move",this._cropDragOrigin={x:e.x,y:e.y},this._cropRectOrigin={...this._cropRect},this._updateCropActions();return}}this._cropRect={x:e.x,y:e.y,w:0,h:0},this._cropDragging=!0,this._cropDragOrigin={x:e.x,y:e.y}}_handleSelectPointerMove(e){if(this._selectionDrawing&&this._startPoint){const t=this._getDocPoint(e),s=this.previewCanvas.getContext("2d");s.clearRect(0,0,this._vw,this._vh);const i=Math.min(this._startPoint.x,t.x),a=Math.min(this._startPoint.y,t.y),o=Math.abs(t.x-this._startPoint.x),r=Math.abs(t.y-this._startPoint.y);s.save(),s.translate(this._panX,this._panY),s.scale(this._zoom,this._zoom),Ya(s,i,a,o,r,0),s.restore()}}_handleSelectPointerUp(e){if(this._selectionDrawing&&this._startPoint){this._selectionDrawing=!1;const t=this._getDocPoint(e),s=Math.min(this._startPoint.x,t.x),i=Math.min(this._startPoint.y,t.y),a=Math.max(this._startPoint.x,t.x),o=Math.max(this._startPoint.y,t.y);this._startPoint=null;const r=Math.max(0,Math.min(this._docWidth,s)),n=Math.max(0,Math.min(this._docHeight,i)),c=Math.max(0,Math.min(this._docWidth,a)),l=Math.max(0,Math.min(this._docHeight,o)),h=c-r,d=l-n;if(h<2||d<2){this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh);return}const p=Math.round(r),f=Math.round(n),u=Math.round(c)-p,_=Math.round(l)-f;if(u<1||_<1){this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh);return}const m=this._ctx.value?.state;if(!m)return;const v=m.layers.find(b=>b.id===m.activeLayerId);if(v&&u>0&&_>0){const b=v.canvas.getContext("2d");this._captureBeforeDraw();const y=b.getImageData(p,f,u,_);b.clearRect(p,f,u,_),this._transformContentMode="lifted",this._transformManager=new rt(y,{x:p,y:f,w:u,h:_},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange()}}}_handleCropPointerMove(e){const t=this._getDocPoint(e),s=Se(this.ctx.state.cropAspectRatio);if(this._cropDragging&&this._cropDragOrigin){let i={x:this._cropDragOrigin.x,y:this._cropDragOrigin.y,w:t.x-this._cropDragOrigin.x,h:t.y-this._cropDragOrigin.y};s&&(i=vs(i,s,"draw")),this._cropRect=i,this._drawCropPreview();return}if(this._cropHandle&&this._cropDragOrigin&&this._cropRectOrigin){const i=t.x-this._cropDragOrigin.x,a=t.y-this._cropDragOrigin.y,o=this._cropRectOrigin;if(this._cropHandle==="move"){let r=o.x+i,n=o.y+a;const c=Math.abs(o.w),l=Math.abs(o.h);r=Math.max(0,Math.min(r,this._docWidth-c)),n=Math.max(0,Math.min(n,this._docHeight-l)),this._cropRect={x:r,y:n,w:c,h:l}}else{let r=this._resizeCropRect(o,this._cropHandle,i,a);s&&(r=vs(r,s,this._cropHandle)),this._cropRect=r}this._drawCropPreview();return}if(this._cropRect){const i=gs(this._cropRect,t,this._zoom);i&&i!=="move"?this.mainCanvas.style.cursor=this._cropHandleCursor(i):i==="move"?this.mainCanvas.style.cursor="move":this.mainCanvas.style.cursor="crosshair"}}_cropHandleCursor(e){return{nw:"nwse-resize",n:"ns-resize",ne:"nesw-resize",e:"ew-resize",se:"nwse-resize",s:"ns-resize",sw:"nesw-resize",w:"ew-resize"}[e]??"crosshair"}_resizeCropRect(e,t,s,i){let{x:a,y:o,w:r,h:n}=e;switch(t){case"nw":a+=s,o+=i,r-=s,n-=i;break;case"n":o+=i,n-=i;break;case"ne":r+=s,o+=i,n-=i;break;case"e":r+=s;break;case"se":r+=s,n+=i;break;case"s":n+=i;break;case"sw":a+=s,r-=s,n+=i;break;case"w":a+=s,r-=s;break}return{x:a,y:o,w:r,h:n}}_handleCropPointerUp(){const e=Se(this.ctx.state.cropAspectRatio);this._cropDragging&&this._cropRect&&(this._cropRect=this._normalizeCropRect(this._cropRect,e),(this._cropRect.w<1||this._cropRect.h<1)&&(this._cropRect=null)),this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._cropRect&&(this._cropRect=this._normalizeCropRect(this._cropRect,e),(this._cropRect.w<1||this._cropRect.h<1)&&(this._cropRect=null)),this._updateCropActions(),this._cropRect?this._drawCropPreview():this._clearCropPreview()}_normalizeCropRect(e,t){let{x:s,y:i,w:a,h:o}=e;a<0&&(s+=a,a=-a),o<0&&(i+=o,o=-o),t===void 0&&o>0&&a>0&&(t=a/o),s<0&&(a+=s,s=0),i<0&&(o+=i,i=0);const r=this._docWidth-s,n=this._docHeight-i,c=a>r,l=o>n;if(a=Math.min(a,r),o=Math.min(o,n),t&&(c||l)){if(c&&l){const u=a/t,_=o*t;u<=n?o=u:_<=r?a=_:a/o>t?a=o*t:o=a/t}else c?o=a/t:a=o*t;a=Math.min(a,this._docWidth-s),o=Math.min(o,this._docHeight-i)}const h=Math.round(s),d=Math.round(i);let p=Math.round(a),f=Math.round(o);return p=Math.min(p,this._docWidth-h),f=Math.min(f,this._docHeight-d),{x:h,y:d,w:p,h:f}}_drawCropPreview(){if(!this.previewCanvas||!this._cropRect)return;const e=this.previewCanvas.getContext("2d");e.clearRect(0,0,this._vw,this._vh),e.save(),e.translate(this._panX,this._panY),e.scale(this._zoom,this._zoom),Xa(e,this._cropRect,this._docWidth,this._docHeight,this._zoom),e.restore()}commitCrop(){if(!this._cropRect)return;const e=this._cropRect;if(e.w<1||e.h<1)return;const t=this._ctx.value?.state;if(!t)return;const s=this._docWidth,i=this._docHeight,a=t.layers.map(r=>{const n=r.canvas.getContext("2d");return{id:r.id,name:r.name,visible:r.visible,opacity:r.opacity,blendMode:r.blendMode,imageData:n.getImageData(0,0,r.canvas.width,r.canvas.height)}});for(const r of t.layers){const c=r.canvas.getContext("2d").getImageData(e.x,e.y,e.w,e.h),l=document.createElement("canvas");l.width=e.w,l.height=e.h,l.getContext("2d").putImageData(c,0,0),r.canvas=l}const o=t.layers.map(r=>{const n=r.canvas.getContext("2d");return{id:r.id,name:r.name,visible:r.visible,opacity:r.opacity,blendMode:r.blendMode,imageData:n.getImageData(0,0,r.canvas.width,r.canvas.height)}});this.dispatchEvent(new CustomEvent("crop-commit",{bubbles:!0,composed:!0,detail:{width:e.w,height:e.h}})),this._pushHistoryEntry({type:"crop",beforeLayers:a,afterLayers:o,beforeWidth:s,beforeHeight:i,afterWidth:e.w,afterHeight:e.h}),this._cropRect=null,this._clearCropPreview(),this.composite()}cancelCrop(){this._cropRect&&(this._cropRect=null,this._cropDragging=!1,this._cropHandle=null,this._cropDragOrigin=null,this._cropRectOrigin=null,this._clearCropPreview())}get hasCropRect(){return this._cropRect!==null}_clearCropPreview(){this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}_createStampAsTransform(e,t,s,i,a=!1){if(e.naturalWidth<=0||e.naturalHeight<=0)return;const o=i/Math.max(e.naturalWidth,e.naturalHeight),r=Math.max(1,Math.round(e.naturalWidth*o)),n=Math.max(1,Math.round(e.naturalHeight*o)),c=Math.round(t-r/2),l=Math.round(s-n/2),h=document.createElement("canvas");h.width=r,h.height=n,h.getContext("2d").drawImage(e,0,0,r,n);const d=h.getContext("2d").getImageData(0,0,r,n);this._transformContentMode="inserted",this._transformManager=new rt(d,{x:c,y:l,w:r,h:n},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this._transformManager.setTouchMode(a),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}async _handleExternalImage(e,t){this._transformManager&&this.commitTransform();let s=e.naturalWidth,i=e.naturalHeight;const a=this._docWidth,o=this._docHeight;if((s>a||i>o)&&await this._resizeDialog.show(s,i,a,o)){const f=Math.min(a/s,o/i);s=Math.round(s*f),i=Math.round(i*f)}this.ctx.addLayer(t),await this.updateComplete,this._captureBeforeDraw(),this._floatIsExternalImage=!0;const r=this._docWidth/2,n=this._docHeight/2,c=Math.round(r-s/2),l=Math.round(n-i/2),h=document.createElement("canvas");h.width=s,h.height=i,h.getContext("2d").drawImage(e,0,0,s,i);const d=h.getContext("2d").getImageData(0,0,s,i);this._transformContentMode="inserted",this._transformManager=new rt(d,{x:c,y:l,w:s,h:i},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}copySelection(){if(!this._transformManager)return;const e=this._transformManager.snapshot(),t=e.canvas.getContext("2d");this.commitTransform(),this._clipboard=t.getImageData(0,0,e.w,e.h),this._clipboardOrigin={x:e.x,y:e.y},this._clipboardRotation=0,this._writeToSystemClipboard(e.canvas),this._notifyHistory()}_writeToSystemClipboard(e){e.toBlob(t=>{t&&(this._clipboardBlobSize=t.size,navigator.clipboard.write([new ClipboardItem({"image/png":t})]).catch(()=>{}))},"image/png")}cutSelection(){if(!this._transformManager)return;this.copySelection();const e=this._clipboardOrigin,t=this._clipboard;if(e&&t){this._captureBeforeDraw();const s=this._getActiveLayerCtx();s?(s.clearRect(e.x,e.y,t.width,t.height),this._pushDrawHistory(!0),this.composite()):this._beforeDrawCanvas=null}}pasteSelection(){if(!this._clipboard||!this._clipboardOrigin)return;this._transformManager&&this.commitTransform(),this._beforeDrawCanvas||this._captureBeforeDraw();const e=this._clipboard.width,t=this._clipboard.height,s=Math.max(0,Math.min(this._clipboardOrigin.x,this._docWidth-1)),i=Math.max(0,Math.min(this._clipboardOrigin.y,this._docHeight-1)),a=new ImageData(new Uint8ClampedArray(this._clipboard.data),e,t);this._transformContentMode="inserted",this._transformManager=new rt(a,{x:s,y:i,w:e,h:t},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this._clipboardRotation&&(this._transformManager.rotation=this._clipboardRotation*180/Math.PI),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}async paste(){try{const e=await navigator.clipboard.read();for(const t of e){const s=t.types.find(r=>r.startsWith("image/"));if(!s)continue;const i=await t.getType(s),a=URL.createObjectURL(i);let o;try{o=await new Promise((r,n)=>{const c=new Image;c.onload=()=>r(c),c.onerror=()=>n(new Error("Image load failed")),c.src=a}),URL.revokeObjectURL(a)}catch{URL.revokeObjectURL(a);continue}if(this._clipboard&&o.naturalWidth===this._clipboard.width&&o.naturalHeight===this._clipboard.height){this.pasteSelection();return}await this._handleExternalImage(o,"Pasted Image");return}}catch{}this.pasteSelection()}selectAll(){this._transformManager&&this.commitTransform();const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(c=>c.id===e.activeLayerId);if(!t)return;const s=t.canvas.getContext("2d"),i=this._docWidth,a=this._docHeight,o=s.getImageData(0,0,i,a),r=xs(o);if(!r)return;this._captureBeforeDraw();const n=s.getImageData(r.x,r.y,r.w,r.h);s.clearRect(r.x,r.y,r.w,r.h),this._transformContentMode="lifted",this._transformManager=new rt(n,r,this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange()}selectAllCanvas(){this._transformManager&&this.commitTransform();const e=this._ctx.value?.state;if(!e)return;const t=e.layers.find(r=>r.id===e.activeLayerId);if(!t)return;const s=t.canvas.getContext("2d"),i=this._docWidth,a=this._docHeight;this._captureBeforeDraw();const o=s.getImageData(0,0,i,a);s.clearRect(0,0,i,a),this._transformContentMode="lifted",this._transformManager=new rt(o,{x:0,y:0,w:i,h:a},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange()}duplicateInPlace(){if(this._transformManager){const e=this._transformManager.snapshot(),t=e.canvas.getContext("2d").getImageData(0,0,e.w,e.h);this._clipboard=new ImageData(new Uint8ClampedArray(t.data),t.width,t.height),this._clipboardOrigin={x:e.x,y:e.y},this._clipboardRotation=0,this._writeToSystemClipboard(e.canvas),this.commitTransform(),this._captureBeforeDraw();const s=new ImageData(new Uint8ClampedArray(t.data),t.width,t.height);this._transformContentMode="inserted",this._transformManager=new rt(s,{x:e.x,y:e.y,w:e.w,h:e.h},this.previewCanvas,this._zoom,{x:this._panX,y:this._panY}),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}else this.pasteSelection()}deleteSelection(){if(this._transformManager){if(this._transformContentMode==="inserted"&&!this._beforeDrawCanvas){this.cancelTransform();return}this._beforeDrawCanvas||this._captureBeforeDraw(),this._transformManager.dispose(),this._transformManager=null,this._transformContentMode="lifted",this._pushDrawHistory(!0),this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite(),this.requestUpdate(),this._dispatchTransformChange(),this._notifyHistory()}}clearSelection(){if(this._textEditing&&this._commitText(),this._cropRect&&this.cancelCrop(),this._drawing){const e=this._getActiveLayerCtx(),t=e?this._commitStroke(e):void 0;this._drawing=!1,this._lastPoint=null,this._startPoint=null,this._pushDrawHistory(!1,t),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this.composite()}this._moveTempCanvas&&(this._moveTempCanvas=null,this._moveStartPoint=null,this._pushDrawHistory(),this.composite()),this._selectionDrawing&&(this._selectionDrawing=!1,this._startPoint=null,this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)),this._transformManager&&this.commitTransform()}cancelExternalFloat(){if(!this._floatIsExternalImage||!this._transformManager)return;const e=this.ctx.state.activeLayerId;if(this._transformManager.dispose(),this._transformManager=null,this._floatIsExternalImage=!1,this._transformContentMode="lifted",this._selectionDrawing=!1,this._beforeDrawCanvas=null,this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh),this._dispatchTransformChange(),this._historyIndex>=0){let t=-1;for(let s=this._historyIndex;s>=0;s--){const i=this._history[s];if(i.type==="add-layer"&&i.layer.id===e){t=s;break}}if(t>=0){const s=this._history.slice(0,t),a=this._history.slice(t,this._historyIndex+1).filter(o=>{const r=this._getEntryLayerId(o);return r===null||r!==e});this._history=[...s,...a],this._historyIndex=this._history.length-1}}this.dispatchEvent(new CustomEvent("layer-undo",{bubbles:!0,composed:!0,detail:{action:"remove-layer",layerId:e}})),this.composite(),this._notifyHistory()}get hasExternalFloat(){return this._floatIsExternalImage&&this._transformManager!==null}getFloatSnapshot(){if(!this._transformManager)return null;const e=this._ctx.value?.state.activeLayerId;if(!e)return null;const t=this._transformManager.snapshot();return{layerId:e,tempCanvas:t.canvas,x:t.x,y:t.y}}connectedCallback(){super.connectedCallback();const e=document.createElement("textarea");e.style.cssText="position:fixed;top:0;left:0;width:1px;height:1px;opacity:0;border:0;padding:0;margin:0;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;",e.setAttribute("autocomplete","off"),e.setAttribute("autocorrect","off"),e.setAttribute("autocapitalize","off"),e.setAttribute("spellcheck","false"),e.setAttribute("aria-label","Text"),e.addEventListener("input",()=>{this._textEditing&&(this._startTextCursorBlink(),this._renderTextPreview())}),e.addEventListener("keydown",t=>this._onTextKeydown(t)),e.addEventListener("selectionchange",()=>{this._textEditing&&(this._startTextCursorBlink(),this._renderTextPreview())}),this._textAreaEl=e,this.addEventListener("wheel",this._onWheel,{passive:!1}),this.addEventListener("dragover",this._onDragOver),this.addEventListener("dragenter",this._onDragEnter),this.addEventListener("dragleave",this._onDragLeave),this.addEventListener("drop",this._onDrop),window.addEventListener("blur",this._onWindowBlur),window.addEventListener("resize",this._invalidateCanvasRect),window.addEventListener("scroll",this._invalidateCanvasRect,!0)}disconnectedCallback(){super.disconnectedCallback(),this._textCursorInterval&&(clearInterval(this._textCursorInterval),this._textCursorInterval=0),this._textAreaEl&&(this._textAreaEl.remove(),this._textAreaEl=null),this._resizeObserver?.disconnect(),this._resizeObserver=null,this._compositeScheduler.cancel(),this._transformManager&&(this._transformManager.dispose(),this._transformManager=null,this._transformContentMode="lifted",this._floatIsExternalImage=!1),this._pointers.clear(),this._pinching=!1,this._panning=!1,this._panPointerId=-1,this.removeEventListener("wheel",this._onWheel),this.removeEventListener("dragover",this._onDragOver),this.removeEventListener("dragenter",this._onDragEnter),this.removeEventListener("dragleave",this._onDragLeave),this.removeEventListener("drop",this._onDrop),window.removeEventListener("blur",this._onWindowBlur),window.removeEventListener("resize",this._invalidateCanvasRect),window.removeEventListener("scroll",this._invalidateCanvasRect,!0)}_renderTextPreview(){if(!this._textEditing||!this._textAreaEl||!this.previewCanvas)return;const e=this.previewCanvas.getContext("2d");e.clearRect(0,0,this._vw,this._vh);const t=this.ctx.state,s=this._textAreaEl.value,{fontFamily:i,fontSize:a,fontBold:o,fontItalic:r,strokeColor:n}=t;e.save(),e.translate(this._panX,this._panY),e.scale(this._zoom,this._zoom),bs(e,s,this._textPosition.x,this._textPosition.y,a,i,o,r,n);const c=ys(e,s,a,i,o,r),l=a*le,h=this._textAreaEl.selectionStart??0,d=this._textAreaEl.selectionEnd??h,p=s.split(`
`);e.font=he(a,i,o,r),e.textBaseline="top";const f=y=>{let x=0;for(let k=0;k<p.length;k++){if(x+p[k].length>=y){const P=y-x,R=p[k].substring(0,P);return{line:k,x:this._textPosition.x+e.measureText(R).width,y:this._textPosition.y+k*l}}x+=p[k].length+1}const C=p.length-1;return{line:C,x:this._textPosition.x+e.measureText(p[C]).width,y:this._textPosition.y+C*l}};if(h!==d){e.fillStyle="rgba(99, 102, 241, 0.3)";const y=f(h),x=f(d);if(y.line===x.line)e.fillRect(y.x,y.y,x.x-y.x,l);else{const C=e.measureText(p[y.line]).width;e.fillRect(y.x,y.y,this._textPosition.x+C-y.x,l);for(let k=y.line+1;k<x.line;k++){const P=e.measureText(p[k]).width;e.fillRect(this._textPosition.x,this._textPosition.y+k*l,P,l)}e.fillRect(this._textPosition.x,x.y,x.x-this._textPosition.x,l)}}else if(this._textCursorVisible){const y=f(h);e.fillStyle=n,e.fillRect(y.x,y.y,2/this._zoom,a)}const u=4/this._zoom,_=this._textPosition.x-u,m=this._textPosition.y-u,v=Math.max(c.width,a)+u*2,b=c.height+u*2;e.strokeStyle="rgba(99, 102, 241, 0.6)",e.lineWidth=1/this._zoom,e.setLineDash([4/this._zoom,4/this._zoom]),e.strokeRect(_,m,v,b),e.restore()}_getTextBoundingBox(){const e=this.ctx.state,t=this._textAreaEl?.value??"",s=this.previewCanvas.getContext("2d"),i=ys(s,t,e.fontSize,e.fontFamily,e.fontBold,e.fontItalic),a=4/this._zoom;return{x:this._textPosition.x-a,y:this._textPosition.y-a,w:Math.max(i.width,e.fontSize)+a*2,h:i.height+a*2}}_pointToTextOffset(e){if(!this._textAreaEl)return 0;const t=this.ctx.state,i=this._textAreaEl.value.split(`
`),{fontSize:a,fontFamily:o,fontBold:r,fontItalic:n}=t,c=a*le,l=this.previewCanvas.getContext("2d");l.save(),l.font=he(a,o,r,n),l.textBaseline="top";const h=e.y-this._textPosition.y;let d=Math.floor(h/c);d=Math.max(0,Math.min(d,i.length-1));const p=i[d],f=e.x-this._textPosition.x;let u=0;for(let m=0;m<=p.length;m++){const v=l.measureText(p.substring(0,m)).width;if(m>0){const y=(l.measureText(p.substring(0,m-1)).width+v)/2;f>=y&&(u=m)}else f<0&&(u=0)}l.restore();let _=0;for(let m=0;m<d;m++)_+=i[m].length+1;return _+u}_startTextCursorBlink(){this._textCursorVisible=!0,this._textCursorInterval&&clearInterval(this._textCursorInterval),this._textCursorInterval=window.setInterval(()=>{this._textCursorVisible=!this._textCursorVisible,this._renderTextPreview()},530)}_stopTextCursorBlink(){this._textCursorInterval&&(clearInterval(this._textCursorInterval),this._textCursorInterval=0),this._textCursorVisible=!1}_onTextKeydown(e){this._textEditing&&(e.key==="Escape"?(e.preventDefault(),e.stopPropagation(),this._textAreaEl&&this._textAreaEl.value.length>0?this._commitText():this._cancelText()):e.key==="Tab"&&e.preventDefault())}_commitText(){if(!this._textEditing||!this._textAreaEl)return;const e=this._textAreaEl.value;if(!e){this._cancelText();return}const t=this.ctx.state;this._captureBeforeDraw();const s=this._getActiveLayerCtx();s?(bs(s,e,this._textPosition.x,this._textPosition.y,t.fontSize,t.fontFamily,t.fontBold,t.fontItalic,t.strokeColor),this._pushDrawHistory(),this.composite()):this._beforeDrawCanvas=null,this._endTextEditing()}_cancelText(){this._endTextEditing()}_endTextEditing(){this._textEditing=!1,this._stopTextCursorBlink(),this._textAreaEl&&(this._textAreaEl.value="",this._textAreaEl.blur()),this.previewCanvas&&this.previewCanvas.getContext("2d").clearRect(0,0,this._vw,this._vh)}render(){return g`
      <canvas
        id="main"
        @pointerenter=${this._onPointerEnter}
        @pointerdown=${this._onPointerDown}
        @pointermove=${this._onPointerMove}
        @pointerup=${this._onPointerUp}
        @pointerleave=${this._onPointerLeave}
        @pointercancel=${this._onPointerCancel}
      ></canvas>
      <canvas
        id="preview"
        style="position:absolute;top:0;left:0;pointer-events:none;"
      ></canvas>
      ${this._cropActionsVisible?g`
        <div class="crop-actions" role="group" aria-label="Crop actions">
          <button class="apply" type="button" title="Apply crop (Enter)" @click=${this._onApplyCropClick}>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
            Apply
          </button>
          <button type="button" title="Cancel crop (Esc)" @click=${this._onCancelCropClick}>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>
            Cancel
          </button>
        </div>
      `:""}
      <resize-dialog></resize-dialog>
    `}};T.styles=yt`
    :host {
      display: block;
      flex: 1;
      overflow: hidden;
      position: relative;
      background: #3a3a3a;
    }

    canvas {
      display: block;
      touch-action: none;
    }

    #main {
      background: transparent;
      cursor: crosshair;
    }

    :host(.drop-target) #main {
      outline: 3px dashed #4a90d9;
      outline-offset: -3px;
    }

    /* Crop has no keyboard on touch devices, so the commit/cancel actions
       live on the canvas instead of behind Enter/Escape. */
    .crop-actions {
      position: absolute;
      left: 50%;
      bottom: 1rem;
      transform: translateX(-50%);
      display: flex;
      gap: 0.5rem;
      padding: 0.375rem;
      border-radius: 0.625rem;
      background: rgba(30, 30, 30, 0.92);
      border: 1px solid #555;
      box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
    }

    .crop-actions button {
      display: inline-flex;
      align-items: center;
      gap: 0.375rem;
      min-height: 44px;
      padding: 0 0.875rem;
      border: 1px solid #555;
      border-radius: 0.5rem;
      background: #3a3a3a;
      color: #ddd;
      font-family: inherit;
      font-size: 0.875rem;
      cursor: pointer;
      touch-action: manipulation;
    }

    .crop-actions button.apply {
      background: #2f7d32;
      border-color: #3f9e43;
      color: #fff;
    }
  `;T.MIN_ZOOM=.1;T.MAX_ZOOM=10;T.ZOOM_STEP=1.1;Zt([fe("#main")],T.prototype,"mainCanvas",2);Zt([fe("#preview")],T.prototype,"previewCanvas",2);Zt([fe("resize-dialog")],T.prototype,"_resizeDialog",2);Zt([M()],T.prototype,"_cropActionsVisible",2);T=Zt([wt("drawing-canvas")],T);var io=Object.defineProperty,ao=Object.getOwnPropertyDescriptor,st=(e,t,s,i)=>{for(var a=i>1?void 0:i?ao(t,s):t,o=e.length-1,r;o>=0;o--)(r=e[o])&&(a=(i?r(t,s,a):r(a))||a);return i&&a&&io(t,s,a),a};let V=class extends F{constructor(){super(...arguments),this._ctx=new gt(this,{context:Lt,subscribe:!0}),this._floatDetail=null,this._thumbnailScheduler=Ne(()=>this._updateThumbnails(),250),this._onComposited=e=>{this._floatDetail=e.detail,this._thumbnailScheduler.schedule()},this._onDocClick=()=>{this._closeContextMenu(),this._dropdownOpen=!1},this._onDocKeyDown=e=>{e.key==="Escape"&&(this._closeContextMenu(),this._dropdownOpen=!1)},this._sheetOpen=!1,this._sheetY=0,this._sheetDragging=!1,this._syncingSheet=!1,this._sheetDragStartY=0,this._sheetDragStartTranslate=0,this._sheetSnapHalf=0,this._sheetSnapFull=0,this._sheetDragTimestamps=[],this._contextMenuOpen=!1,this._contextMenuX=0,this._contextMenuY=0,this._dropdownOpen=!1,this._editingLayerId=null,this._draggedLayerId=null,this._dragPointerId=null,this._dragStartY=0,this._dragCurrentY=0,this._dragThreshold=5,this._dragActivated=!1,this._opacityBefore=null,this._eyeOpen=g`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>`,this._eyeClosed=g`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/><line x1="1" y1="1" x2="23" y2="23"/></svg>`,this._plusIcon=g`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>`,this._trashIcon=g`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/></svg>`,this._dragTarget=null,this._onResize=()=>{this._sheetOpen&&!this._sheetDragging&&this._recalcSnapPoints()},this._lastThumbLayers=null,this._lastThumbRowCount=-1,this._miniCheckerboardPattern=null}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback(),this.getRootNode().addEventListener("composited",this._onComposited),window.addEventListener("resize",this._onResize),document.addEventListener("click",this._onDocClick),document.addEventListener("keydown",this._onDocKeyDown)}disconnectedCallback(){super.disconnectedCallback(),this.getRootNode().removeEventListener("composited",this._onComposited),this._thumbnailScheduler.cancel(),window.removeEventListener("resize",this._onResize),document.removeEventListener("click",this._onDocClick),document.removeEventListener("keydown",this._onDocKeyDown)}_toggleVisibility(e,t){t.stopPropagation(),this.ctx.setLayerVisibility(e.id,!e.visible)}_startRename(e,t){t.stopPropagation(),this._editingLayerId=e}_commitRename(e,t){if(this._editingLayerId!==e)return;const s=t.value.trim();s&&s!==this._getLayerById(e)?.name&&this.ctx.renameLayer(e,s),this._editingLayerId=null}_onRenameKeyDown(e,t){t.stopPropagation(),t.key==="Enter"?this._commitRename(e,t.target):t.key==="Escape"&&(this._editingLayerId=null)}_onRenameBlur(e,t){this._commitRename(e,t.target)}_moveUp(e,t){t.stopPropagation();const s=this.ctx.state.layers,i=s.findIndex(a=>a.id===e.id);i<s.length-1&&this.ctx.reorderLayer(e.id,i+1)}_moveDown(e,t){t.stopPropagation();const i=this.ctx.state.layers.findIndex(a=>a.id===e.id);i>0&&this.ctx.reorderLayer(e.id,i-1)}_onReorderPointerDown(e,t){t.button===0&&(this._draggedLayerId=e.id,this._dragPointerId=t.pointerId,this._dragStartY=t.clientY,this._dragCurrentY=t.clientY,this._dragActivated=!1,this._dragTarget=t.currentTarget)}_onReorderPointerMove(e){if(this._dragPointerId!==e.pointerId||!this._draggedLayerId)return;if(this._dragCurrentY=e.clientY,!this._dragActivated){if(Math.abs(this._dragCurrentY-this._dragStartY)<this._dragThreshold)return;this._dragActivated=!0,this._dragTarget&&this._dragTarget.setPointerCapture(e.pointerId)}this._clearDropIndicators();const t=this.shadowRoot?.querySelectorAll(".layer-row");if(t)for(const s of t){const i=s.getBoundingClientRect();if(this._dragCurrentY>=i.top&&this._dragCurrentY<=i.bottom){const a=i.top+i.height/2;this._dragCurrentY<a?s.classList.add("drop-above"):s.classList.add("drop-below");break}}}_onReorderPointerUp(e){if(this._dragPointerId!==e.pointerId)return;const t=this._draggedLayerId;if(!t||!this._dragActivated){this._clearDragState();return}const s=this.shadowRoot?.querySelectorAll(".layer-row");if(!s){this._clearDragState();return}let i=null,a=!1;for(const o of s){const r=o.getBoundingClientRect();if(this._dragCurrentY>=r.top&&this._dragCurrentY<=r.bottom){i=o.dataset.layerId??null;const n=r.top+r.height/2;a=this._dragCurrentY<n;break}}if(i&&i!==t){const o=this.ctx.state.layers,r=o.findIndex(n=>n.id===i);if(r!==-1){let n=a?r+1:r;const c=o.findIndex(l=>l.id===t);c<n&&(n-=1),n=Math.max(0,Math.min(o.length-1,n)),c!==n&&this.ctx.reorderLayer(t,n)}}this._clearDragState()}_onReorderPointerCancel(e){this._clearDragState()}_clearDropIndicators(){this.shadowRoot?.querySelectorAll(".layer-row")?.forEach(t=>t.classList.remove("drop-above","drop-below"))}_clearDragState(){this._draggedLayerId=null,this._dragPointerId=null,this._dragActivated=!1,this._dragTarget=null,this._clearDropIndicators()}openSheet(){this._sheetSnapFull=0,this._sheetY=0,this._sheetOpen=!0,this.updateComplete.then(()=>this._measureSnaps())}_measureSnaps(){const e=this.shadowRoot?.querySelector(".sheet"),t=e?e.offsetHeight:window.innerHeight*.9;this._sheetSnapHalf=Math.floor(t*.5)}closeSheet(){this._sheetOpen=!1,this._sheetY=0,!this._syncingSheet&&this.ctx?.state.layersPanelOpen&&this.ctx.toggleLayersPanel()}_recalcSnapPoints(){const e=this.shadowRoot?.querySelector(".sheet"),t=e?e.offsetHeight:window.innerHeight*.9,s=this._sheetSnapHalf;if(this._sheetSnapFull=0,this._sheetSnapHalf=Math.floor(t*.5),s>0){if(this._sheetY===s)this._sheetY=this._sheetSnapHalf;else if(this._sheetY!==this._sheetSnapFull){const i=Math.abs(this._sheetY-this._sheetSnapHalf),a=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=i<a?this._sheetSnapHalf:this._sheetSnapFull}}}_onSheetHandlePointerDown(e){this._sheetDragging=!0,this._sheetDragStartY=e.clientY,this._sheetDragStartTranslate=this._sheetY,this._sheetDragTimestamps=[{y:e.clientY,t:Date.now()}],e.target.setPointerCapture(e.pointerId)}_onSheetHandlePointerMove(e){if(!this._sheetDragging)return;const t=e.clientY-this._sheetDragStartY,s=Math.max(this._sheetSnapFull,this._sheetDragStartTranslate+t);this._sheetY=s,this._sheetDragTimestamps.push({y:e.clientY,t:Date.now()}),this._sheetDragTimestamps.length>5&&this._sheetDragTimestamps.shift()}_onSheetHandlePointerUp(e){if(!this._sheetDragging)return;this._sheetDragging=!1;const t=this._sheetDragTimestamps;let s=0;if(t.length>=2){const r=t[t.length-1],n=t[t.length-2],c=r.t-n.t;c>0&&(s=(r.y-n.y)/c)}const i=window.innerHeight*.75;if(this._sheetY>i||s>.5){this.closeSheet();return}const a=Math.abs(this._sheetY-this._sheetSnapHalf),o=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=a<o?this._sheetSnapHalf:this._sheetSnapFull}_onSheetHandlePointerCancel(e){if(!this._sheetDragging)return;this._sheetDragging=!1;const t=Math.abs(this._sheetY-this._sheetSnapHalf),s=Math.abs(this._sheetY-this._sheetSnapFull);this._sheetY=t<s?this._sheetSnapHalf:this._sheetSnapFull}_onOpacityPointerDown(e){this._opacityBefore=e.opacity}_onOpacityInput(e,t){if(this._opacityBefore===null){const i=this._getLayerById(e);i&&(this._opacityBefore=i.opacity)}const s=Number(t.target.value)/100;this.ctx.setLayerOpacity(e,s)}_onOpacityChange(e,t){const s=Number(t.target.value)/100,i=this._opacityBefore;this._opacityBefore=null,i!==null&&i!==s&&this.dispatchEvent(new CustomEvent("commit-opacity",{bubbles:!0,composed:!0,detail:{layerId:e,before:i,after:s}}))}_onContextMenu(e,t){e.preventDefault(),this._selectLayer(t),this._contextMenuX=e.clientX,this._contextMenuY=e.clientY,this._contextMenuOpen=!0}_closeContextMenu(){this._contextMenuOpen=!1}_getLayerById(e){return this.ctx.state.layers.find(t=>t.id===e)}_selectLayer(e){this.ctx.setActiveLayer(e)}render(){if(!this._ctx.value)return g``;if(this.ctx.isMobile)return this._renderMobileSheet();const{layersPanelOpen:e}=this.ctx.state;return e?g`
      <div class="panel">
        ${this._renderLayersList()}
      </div>
    `:g`
        <div class="panel collapsed">
          <div class="collapsed-strip">
            <button
              class="expand-btn"
              title="Show layers"
              @click=${()=>this.ctx.toggleLayersPanel()}
            >&#9654;</button>
            <span class="vertical-label">Layers</span>
          </div>
        </div>
      `}_renderMobileSheet(){const e=this._sheetOpen?`transform: translateY(${this._sheetY}px);${this._sheetDragging?"transition:none;":""}`:"transform: translateY(100%);";return g`
      <div
        class="sheet-backdrop ${this._sheetOpen?"open":""}"
        @click=${()=>this.closeSheet()}
      ></div>
      <div
        class="sheet ${this._sheetOpen?"open":""}"
        style=${e}
      >
        <div
          class="sheet-handle"
          @pointerdown=${t=>this._onSheetHandlePointerDown(t)}
          @pointermove=${t=>this._onSheetHandlePointerMove(t)}
          @pointerup=${t=>this._onSheetHandlePointerUp(t)}
          @pointercancel=${t=>this._onSheetHandlePointerCancel(t)}
        >
          <div class="sheet-handle-bar"></div>
        </div>
        <div class="sheet-content">
          ${this._renderLayersList()}
        </div>
      </div>
    `}_renderLayersList(){const{layers:e,activeLayerId:t}=this.ctx.state,s=[...e].reverse();return g`
      <div class="header">
        <span class="header-title">Layers</span>
        <button
          class="collapse-btn"
          title="Hide layers"
          @click=${()=>this.ctx.toggleLayersPanel()}
        >&#9664; hide</button>
      </div>

      <div class="layer-list">
        ${s.map(i=>this._renderLayerRow(i,e,t))}
      </div>

      ${this._contextMenuOpen?g`
        <div
          class="context-menu"
          style="left:${this._contextMenuX}px;top:${this._contextMenuY}px"
          @click=${i=>i.stopPropagation()}
        >
          <button
            class="context-menu-item"
            ?disabled=${e.findIndex(i=>i.id===t)===0}
            @click=${()=>{this.ctx.mergeLayerDown(t),this._closeContextMenu()}}
          >Merge Down</button>
          <button
            class="context-menu-item"
            ?disabled=${e.filter(i=>i.visible).length<2}
            @click=${()=>{this.ctx.mergeVisibleLayers(),this._closeContextMenu()}}
          >Merge Visible</button>
          <button
            class="context-menu-item"
            ?disabled=${e.length<=1}
            @click=${()=>{this.ctx.flattenImage(),this._closeContextMenu()}}
          >Flatten Image</button>
        </div>
      `:S}

      <div class="action-bar-wrapper">
        ${this._dropdownOpen?g`
          <div class="dropdown-menu" @click=${i=>i.stopPropagation()}>
            <button
              ?disabled=${e.findIndex(i=>i.id===t)===0}
              @click=${()=>{this.ctx.mergeLayerDown(t),this._dropdownOpen=!1}}
            >Merge Down</button>
            <button
              ?disabled=${e.filter(i=>i.visible).length<2}
              @click=${()=>{this.ctx.mergeVisibleLayers(),this._dropdownOpen=!1}}
            >Merge Visible</button>
            <button
              ?disabled=${e.length<=1}
              @click=${()=>{this.ctx.flattenImage(),this._dropdownOpen=!1}}
            >Flatten Image</button>
          </div>
        `:S}
        <div class="action-bar">
          <button
            class="action-btn"
            title="Add layer"
            @click=${()=>this.ctx.addLayer()}
          >${this._plusIcon} Add</button>
          <button
            class="action-btn"
            title="Delete layer"
            ?disabled=${e.length<=1}
            @click=${()=>this.ctx.deleteLayer(t)}
          >${this._trashIcon} Delete</button>
          <button
            class="action-btn"
            title="More actions"
            @click=${i=>{i.stopPropagation(),this._dropdownOpen=!this._dropdownOpen}}
          >&#8943;</button>
        </div>
      </div>
    `}_renderLayerRow(e,t,s){const i=e.id===s,a=t.findIndex(c=>c.id===e.id),o=a===t.length-1,r=a===0,n=this._editingLayerId===e.id;return g`
      <div
        class="layer-row ${i?"active":""} ${this._draggedLayerId===e.id?"dragging":""}"
        data-layer-id=${e.id}
        @click=${()=>this._selectLayer(e.id)}
        @contextmenu=${c=>this._onContextMenu(c,e.id)}
        @pointerdown=${c=>this._onReorderPointerDown(e,c)}
        @pointermove=${c=>this._onReorderPointerMove(c)}
        @pointerup=${c=>this._onReorderPointerUp(c)}
        @pointercancel=${c=>this._onReorderPointerCancel(c)}
      >
        <div
          class="layer-row-main"
        >
          <button
            class="vis-btn ${e.visible?"":"hidden"}"
            title=${e.visible?"Hide layer":"Show layer"}
            @click=${c=>this._toggleVisibility(e,c)}
          >
            ${e.visible?this._eyeOpen:this._eyeClosed}
          </button>

          <canvas class="layer-thumb" width="48" height="36"></canvas>

          ${n?g`<input
                class="layer-name-input"
                aria-label="Layer name"
                .value=${e.name}
                @keydown=${c=>this._onRenameKeyDown(e.id,c)}
                @blur=${c=>this._onRenameBlur(e.id,c)}
                @click=${c=>c.stopPropagation()}
                ${this._autoFocusDirective()}
              />`:g`<span
                class="layer-name"
                @dblclick=${c=>this._startRename(e.id,c)}
              >${e.name}</span>`}

          ${this.ctx.isMobile?g`
            <button
              class="rename-btn"
              title="Rename"
              @click=${c=>{c.stopPropagation(),this._startRename(e.id,c)}}
            >
              <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M11.5 1.5l3 3L5 14H2v-3L11.5 1.5z"/>
              </svg>
            </button>
          `:""}

          <div class="reorder-btns">
            <button
              class="reorder-btn"
              title="Move up"
              ?disabled=${o}
              @click=${c=>this._moveUp(e,c)}
            >&#9650;</button>
            <button
              class="reorder-btn"
              title="Move down"
              ?disabled=${r}
              @click=${c=>this._moveDown(e,c)}
            >&#9660;</button>
          </div>
        </div>

        ${i?g`
            <div class="opacity-row">
              <select class="blend-mode-select"
                aria-label=${`Blend mode of ${e.name}`}
                .value=${e.blendMode}
                @change=${c=>this.ctx.setLayerBlendMode(e.id,c.target.value)}>
                ${Object.entries(xi).map(([c,l])=>g`
                  <option value=${c}>${l}</option>
                `)}
              </select>
            </div>
            <div class="opacity-row">
              <input
                type="range"
                min="0"
                max="100"
                aria-label=${`Opacity of ${e.name}`}
                .value=${String(Math.round(e.opacity*100))}
                @pointerdown=${()=>this._onOpacityPointerDown(e)}
                @input=${c=>this._onOpacityInput(e.id,c)}
                @change=${c=>this._onOpacityChange(e.id,c)}
              />
              <span class="opacity-value">${Math.round(e.opacity*100)}%</span>
            </div>
          `:S}
      </div>
    `}_autoFocusDirective(){return requestAnimationFrame(()=>{const e=this.shadowRoot?.querySelector(".layer-name-input");e&&(e.focus(),e.select())}),S}willUpdate(){this.ctx?.isMobile&&!this._syncingSheet&&(this._syncingSheet=!0,this.ctx.state.layersPanelOpen&&!this._sheetOpen?this.openSheet():!this.ctx.state.layersPanelOpen&&this._sheetOpen&&this.closeSheet(),this._syncingSheet=!1)}updated(e){super.updated(e);const t=this._ctx.value?.state.layers??null,s=this.shadowRoot?.querySelectorAll(".layer-thumb").length??0;t!==this._lastThumbLayers||s!==this._lastThumbRowCount?(this._lastThumbLayers=t,this._lastThumbRowCount=s,this._updateThumbnails()):this._thumbnailScheduler.schedule()}_updateThumbnails(){const e=this._ctx.value?.state.layers??[],t=this.shadowRoot?.querySelectorAll(".layer-thumb");if(!t)return;const s=[...e].reverse();t.forEach((i,a)=>{const o=s[a];if(!o)return;const r=i.getContext("2d");if(r.clearRect(0,0,i.width,i.height),this._drawMiniCheckerboard(r,i.width,i.height),r.globalAlpha=o.opacity,r.drawImage(o.canvas,0,0,i.width,i.height),this._floatDetail&&o.id===this._floatDetail.layerId){const{tempCanvas:n,rect:c,rotation:l}=this._floatDetail,h=o.canvas.width,d=o.canvas.height,p=c.x/h*i.width,f=c.y/d*i.height,u=c.w/h*i.width,_=c.h/d*i.height;if(l){const m=p+u/2,v=f+_/2;r.save(),r.translate(m,v),r.rotate(l),r.drawImage(n,-u/2,-_/2,u,_),r.restore()}else r.drawImage(n,p,f,u,_)}r.globalAlpha=1})}_drawMiniCheckerboard(e,t,s){if(!this._miniCheckerboardPattern){const a=document.createElement("canvas");a.width=8,a.height=8;const o=a.getContext("2d");o.fillStyle="#ffffff",o.fillRect(0,0,8,8),o.fillStyle="#e0e0e0",o.fillRect(4,0,4,4),o.fillRect(0,4,4,4),this._miniCheckerboardPattern=e.createPattern(a,"repeat")}e.fillStyle=this._miniCheckerboardPattern,e.fillRect(0,0,t,s)}};V.styles=yt`
    :host {
      display: flex;
      flex-direction: column;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 0.8125rem;
      color: #ddd;
      user-select: none;
    }

    /* ── Panel (expanded) ─────────────────────── */
    .panel {
      display: flex;
      flex-direction: column;
      flex: 1;
      overflow: hidden;
    }

    .panel.collapsed {
    }

    /* ── Collapsed strip ──────────────────────── */
    .collapsed-strip {
      display: flex;
      flex-direction: column;
      align-items: center;
      width: 32px;
      height: 100%;
      padding-top: 8px;
      gap: 4px;
    }

    .expand-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      font-size: 0.75rem;
      padding: 0;
    }

    .expand-btn:hover {
      background: #444;
      color: #fff;
    }

    .vertical-label {
      writing-mode: vertical-rl;
      text-orientation: mixed;
      color: #888;
      font-size: 0.6875rem;
      letter-spacing: 0.05em;
      margin-top: 6px;
    }

    /* ── Header ───────────────────────────────── */
    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 6px 8px;
      border-bottom: 1px solid #444;
      flex-shrink: 0;
    }

    .header-title {
      font-weight: 600;
      font-size: 0.8125rem;
    }

    .collapse-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 2px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #aaa;
      cursor: pointer;
      font-size: 0.6875rem;
      padding: 2px 6px;
      white-space: nowrap;
    }

    .collapse-btn:hover {
      background: #444;
      color: #fff;
    }

    /* ── Layer list ────────────────────────────── */
    .layer-list {
      flex: 1;
      overflow-y: auto;
      overflow-x: hidden;
      scrollbar-width: thin;
      scrollbar-color: #555 transparent;
    }

    /* ── Layer row ─────────────────────────────── */
    .layer-row {
      position: relative;
      display: flex;
      flex-direction: column;
      padding: 6px 8px;
      min-height: 44px;
      background: #3a3a3a;
      border-bottom: 1px solid #333;
      cursor: pointer;
      transition: background 0.1s ease;
      touch-action: none;
    }

    .layer-row.dragging {
      opacity: 0.4;
    }

    .layer-row.drop-above::before {
      content: '';
      position: absolute;
      top: -1px;
      left: 0;
      right: 0;
      height: 2px;
      background: #5b8cf7;
    }

    .layer-row.drop-below::after {
      content: '';
      position: absolute;
      bottom: -1px;
      left: 0;
      right: 0;
      height: 2px;
      background: #5b8cf7;
    }

    .layer-row:hover {
      background: #424242;
    }

    .layer-row.active {
      background: #3a3a5c;
    }

    .layer-row-main {
      display: flex;
      align-items: center;
      gap: 4px;
      min-height: 24px;
      cursor: grab;
    }

    .layer-row-main:active {
      cursor: grabbing;
    }

    /* ── Visibility button ─────────────────────── */
    .vis-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 22px;
      height: 22px;
      border: none;
      border-radius: 4px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .vis-btn:hover {
      background: #555;
      color: #fff;
    }

    .vis-btn.hidden {
      color: #666;
    }

    .vis-btn svg {
      width: 14px;
      height: 14px;
    }

    /* ── Layer thumbnail ──────────────────────── */
    .layer-thumb {
      width: 48px;
      height: 36px;
      border-radius: 3px;
      border: 1px solid #555;
      flex-shrink: 0;
    }

    /* ── Layer name ─────────────────────────────── */
    .layer-name {
      flex: 1;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: 0.75rem;
      line-height: 22px;
    }

    .layer-name-input {
      flex: 1;
      min-width: 0;
      font-size: 0.75rem;
      background: #222;
      border: 1px solid #5b8cf7;
      border-radius: 3px;
      color: #ddd;
      padding: 1px 4px;
      outline: 2px solid transparent;
      font-family: inherit;
    }

    /* ── Reorder buttons ───────────────────────── */
    .reorder-btns {
      display: flex;
      flex-direction: column;
      gap: 0px;
      flex-shrink: 0;
    }

    .reorder-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 16px;
      height: 12px;
      border: none;
      border-radius: 3px;
      background: transparent;
      color: #888;
      cursor: pointer;
      font-size: 0.5rem;
      padding: 0;
      line-height: 1;
    }

    .reorder-btn:hover:not(:disabled) {
      background: #555;
      color: #fff;
    }

    .reorder-btn:disabled {
      opacity: 0.25;
      cursor: default;
    }

    /* ── Opacity slider ────────────────────────── */
    .opacity-row {
      display: flex;
      align-items: center;
      gap: 6px;
      margin-top: 4px;
      padding-left: 26px;
    }

    .opacity-row input[type="range"] {
      flex: 1;
      height: 4px;
      accent-color: #5b8cf7;
      min-width: 0;
    }

    .opacity-value {
      font-size: 0.6875rem;
      color: #aaa;
      min-width: 28px;
      text-align: right;
    }

    /* ── Action bar ────────────────────────────── */
    .action-bar {
      display: flex;
      justify-content: center;
      gap: 6px;
      padding: 6px 8px;
      border-top: 1px solid #444;
      flex-shrink: 0;
    }

    .action-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 4px;
      border: none;
      border-radius: 6px;
      background: transparent;
      color: #bbb;
      cursor: pointer;
      font-size: 0.6875rem;
      padding: 4px 10px;
      font-family: inherit;
      transition: all 0.15s ease;
    }

    .action-btn:hover:not(:disabled) {
      background: #444;
      color: #fff;
    }

    .action-btn:disabled {
      opacity: 0.3;
      cursor: default;
    }

    .action-btn svg {
      width: 14px;
      height: 14px;
    }

    /* ── Context menu ──────────────────────────── */
    .context-menu {
      position: fixed;
      z-index: 300;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 6px;
      padding: 4px 0;
      min-width: 160px;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
    }

    .context-menu-item {
      display: block;
      width: 100%;
      padding: 6px 14px;
      border: none;
      background: transparent;
      color: #ddd;
      font-size: 0.8125rem;
      font-family: inherit;
      text-align: left;
      cursor: pointer;
    }

    .context-menu-item:hover:not(:disabled) {
      background: #5b8cf7;
      color: #fff;
    }

    .context-menu-item:disabled {
      color: #666;
      cursor: default;
    }

    /* ── Dropdown menu ─────────────────────────── */
    .action-bar-wrapper {
      position: relative;
    }

    .dropdown-menu {
      position: absolute;
      bottom: 100%;
      right: 0;
      margin-bottom: 4px;
      background: #2a2a2a;
      border: 1px solid #555;
      border-radius: 6px;
      padding: 4px 0;
      min-width: 160px;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
      z-index: 300;
    }

    .dropdown-menu button {
      display: block;
      width: 100%;
      padding: 6px 14px;
      border: none;
      background: transparent;
      color: #ddd;
      font-size: 0.8125rem;
      font-family: inherit;
      text-align: left;
      cursor: pointer;
    }

    .dropdown-menu button:hover:not(:disabled) {
      background: #5b8cf7;
      color: #fff;
    }

    .dropdown-menu button:disabled {
      color: #666;
      cursor: default;
    }

    /* ── Mobile bottom sheet ───────────────────── */
    .sheet-backdrop {
      display: none;
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.4);
      z-index: 200;
    }

    .sheet-backdrop.open {
      display: block;
    }

    .sheet {
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      max-height: 90vh;
      background: #2c2c2c;
      border-radius: 16px 16px 0 0;
      z-index: 201;
      display: flex;
      flex-direction: column;
      transition: transform 0.3s ease;
      transform: translateY(100%);
      will-change: transform;
      padding-bottom: env(safe-area-inset-bottom);
    }

    .sheet-handle {
      display: flex;
      justify-content: center;
      padding: 8px 0;
      cursor: grab;
      touch-action: none;
    }

    .sheet-handle-bar {
      width: 36px;
      height: 4px;
      border-radius: 2px;
      background: #666;
    }

    .sheet-content {
      flex: 1;
      overflow-y: auto;
      touch-action: pan-y;
      -webkit-overflow-scrolling: touch;
    }

    .rename-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 28px;
      height: 28px;
      border: none;
      border-radius: 4px;
      background: transparent;
      color: #888;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .rename-btn:hover {
      background: #444;
      color: #ddd;
    }

    .blend-mode-select {
      width: 100%;
      background: #444;
      color: #ddd;
      border: 1px solid #555;
      border-radius: 4px;
      padding: 2px 4px;
      font-size: 0.75rem;
      cursor: pointer;
    }
  `;st([M()],V.prototype,"_sheetOpen",2);st([M()],V.prototype,"_sheetY",2);st([M()],V.prototype,"_contextMenuOpen",2);st([M()],V.prototype,"_contextMenuX",2);st([M()],V.prototype,"_contextMenuY",2);st([M()],V.prototype,"_dropdownOpen",2);st([M()],V.prototype,"_editingLayerId",2);st([M()],V.prototype,"_draggedLayerId",2);V=st([wt("layers-panel")],V);var oo=Object.defineProperty,ro=Object.getOwnPropertyDescriptor,me=(e,t,s,i)=>{for(var a=i>1?void 0:i?ro(t,s):t,o=e.length-1,r;o>=0;o--)(r=e[o])&&(a=(i?r(t,s,a):r(a))||a);return i&&a&&oo(t,s,a),a};let j=class extends F{constructor(){super(...arguments),this._ctx=new gt(this,{context:Lt,subscribe:!0}),this._minimapCanvas=document.createElement("canvas"),this._isFullscreen=!1,this._onFullscreenChange=()=>{this._isFullscreen=!!document.fullscreenElement},this._toggleFullscreen=()=>{document.fullscreenElement?document.exitFullscreen():document.documentElement.requestFullscreen()},this._minimapScheduler=Ne(()=>this._renderMinimap(),100),this._onComposited=()=>{this._minimapScheduler.schedule()},this._minimapScale=1,this._minimapOffsetX=0,this._minimapOffsetY=0,this._dragging=!1,this._dragOffsetX=0,this._dragOffsetY=0,this._onMinimapPointerDown=e=>{if(e.button!==0)return;const t=this._getMinimapPoint(e);if(this._isInsideViewportRect(t.x,t.y)){const s=this._getViewportRectInMinimap();this._dragging=!0,this._dragOffsetX=t.x-s.x,this._dragOffsetY=t.y-s.y,this._minimapCanvas.setPointerCapture(e.pointerId)}else{this._panToMinimapPoint(t.x,t.y);const s=this._getViewportRectInMinimap();this._dragging=!0,this._dragOffsetX=s.w/2,this._dragOffsetY=s.h/2,this._minimapCanvas.setPointerCapture(e.pointerId)}},this._onMinimapPointerMove=e=>{if(!this._dragging)return;const t=this._getMinimapPoint(e),s=t.x-this._dragOffsetX,i=t.y-this._dragOffsetY,{zoom:a}=this.ctx,o=this._minimapScale,r=(s-this._minimapOffsetX)/o,n=(i-this._minimapOffsetY)/o,c=-r*a,l=-n*a;this.dispatchEvent(new CustomEvent("navigator-pan",{bubbles:!0,composed:!0,detail:{panX:c,panY:l}}))},this._onMinimapPointerUp=e=>{this._dragging&&(this._dragging=!1,this._minimapCanvas.releasePointerCapture(e.pointerId))},this._onSliderInput=e=>{const t=parseInt(e.target.value,10);this._dispatchZoom(this._sliderToZoom(t))},this._onZoomIn=()=>{this._dispatchZoom(this.ctx.zoom*j.ZOOM_STEP)},this._onZoomOut=()=>{this._dispatchZoom(this.ctx.zoom/j.ZOOM_STEP)},this._editingZoom=!1,this._zoomInputValue="",this._onZoomInputFocus=e=>{this._editingZoom=!0,this._zoomInputValue=Math.round(this.ctx.zoom*100).toString();const t=e.target;requestAnimationFrame(()=>t.select())},this._onZoomInputBlur=()=>{this._commitZoomInput(),this._editingZoom=!1},this._onZoomInputKeydown=e=>{e.key==="Enter"?(this._commitZoomInput(),this._editingZoom=!1,e.target.blur()):e.key==="Escape"&&(this._editingZoom=!1,e.target.blur()),e.stopPropagation()},this._onZoomInputChange=e=>{this._zoomInputValue=e.target.value}}get ctx(){return this._ctx.value}connectedCallback(){super.connectedCallback(),this.getRootNode().addEventListener("composited",this._onComposited),document.addEventListener("fullscreenchange",this._onFullscreenChange)}disconnectedCallback(){super.disconnectedCallback(),this.getRootNode().removeEventListener("composited",this._onComposited),this._minimapScheduler.cancel(),document.removeEventListener("fullscreenchange",this._onFullscreenChange)}_renderMinimap(){if(!this._ctx.value)return;const{state:e}=this.ctx,{layers:t,documentWidth:s,documentHeight:i}=e,a=this._minimapCanvas,o=this.shadowRoot?.querySelector(".minimap-container");if(!o)return;const r=o.clientWidth-12;if(r<=0)return;const n=150,c=s/i;let l=r,h=l/c;h>n&&(h=n,l=h*c);const d=window.devicePixelRatio||1,p=Math.round(l*d),f=Math.round(h*d);(a.width!==p||a.height!==f)&&(a.width=p,a.height=f),a.style.width=`${l}px`,a.style.height=`${h}px`;const u=a.getContext("2d");u.setTransform(d,0,0,d,0,0);const _=Math.min(l/s,h/i);this._minimapScale=_;const m=s*_,v=i*_;this._minimapOffsetX=(l-m)/2,this._minimapOffsetY=(h-v)/2,u.fillStyle="#3a3a3a",u.fillRect(0,0,l,h),u.fillStyle="#ffffff",u.fillRect(this._minimapOffsetX,this._minimapOffsetY,m,v),u.save(),u.translate(this._minimapOffsetX,this._minimapOffsetY);const b=t.some(y=>y.visible&&y.blendMode!=="normal");for(const y of t)y.visible&&(u.globalAlpha=y.opacity,b&&(u.globalCompositeOperation=Yt(y.blendMode)),u.drawImage(y.canvas,0,0,m,v));u.globalAlpha=1,u.globalCompositeOperation="source-over",u.restore(),this._drawViewportRect(u,_,l,h)}_drawViewportRect(e,t,s,i){const{zoom:a,panX:o,panY:r,viewportWidth:n,viewportHeight:c}=this.ctx,l=this._minimapOffsetX+-o/a*t,h=this._minimapOffsetY+-r/a*t,d=n/a*t,p=c/a*t,f=Math.max(0,l),u=Math.max(0,h),_=Math.min(s-f,d-(f-l)),m=Math.min(i-u,p-(u-h));_<=0||m<=0||(e.fillStyle="rgba(255, 68, 68, 0.1)",e.fillRect(f,u,_,m),e.strokeStyle="#ff4444",e.lineWidth=1.5,e.strokeRect(f,u,_,m))}_getMinimapPoint(e){const t=this._minimapCanvas.getBoundingClientRect();return{x:e.clientX-t.left,y:e.clientY-t.top}}_getViewportRectInMinimap(){const{zoom:e,panX:t,panY:s,viewportWidth:i,viewportHeight:a}=this.ctx,o=this._minimapScale;return{x:this._minimapOffsetX+-t/e*o,y:this._minimapOffsetY+-s/e*o,w:i/e*o,h:a/e*o}}_isInsideViewportRect(e,t){const s=this._getViewportRectInMinimap();return e>=s.x&&e<=s.x+s.w&&t>=s.y&&t<=s.y+s.h}_panToMinimapPoint(e,t){const{zoom:s,viewportWidth:i,viewportHeight:a}=this.ctx,o=this._minimapScale,r=(e-this._minimapOffsetX)/o,n=(t-this._minimapOffsetY)/o,c=i/2-r*s,l=a/2-n*s;this.dispatchEvent(new CustomEvent("navigator-pan",{bubbles:!0,composed:!0,detail:{panX:c,panY:l}}))}_zoomToSlider(e){const{MIN_ZOOM:t,MAX_ZOOM:s,SLIDER_MAX:i}=j,a=Math.log(e/t)/Math.log(s/t);return Math.round(a*i)}_sliderToZoom(e){const{MIN_ZOOM:t,MAX_ZOOM:s,SLIDER_MAX:i}=j,a=e/i;return t*Math.pow(s/t,a)}_dispatchZoom(e){const t=Math.min(j.MAX_ZOOM,Math.max(j.MIN_ZOOM,e));this.dispatchEvent(new CustomEvent("navigator-zoom",{bubbles:!0,composed:!0,detail:{zoom:t}}))}_commitZoomInput(){const e=this._zoomInputValue.replace("%","").trim(),t=parseFloat(e);if(isNaN(t)||t<=0)return;const s=t/100;this._dispatchZoom(s)}render(){if(!this._ctx.value)return g``;if(!this.ctx.state.layersPanelOpen)return g``;if(this.ctx.isMobile)return g``;const e=this.ctx.zoom,t=this._zoomToSlider(e),s=Math.round(e*100),i=this._editingZoom?this._zoomInputValue:`${s}%`;return g`
      <div class="section">
        <div class="header">
          <span class="header-title">Navigator</span>
        </div>
        <div class="minimap-container"
          @pointerdown=${this._onMinimapPointerDown}
          @pointermove=${this._onMinimapPointerMove}
          @pointerup=${this._onMinimapPointerUp}
        >
          ${this._minimapCanvas}
        </div>
        <div class="zoom-controls">
          <button class="zoom-btn" title="Zoom out" @click=${this._onZoomOut}>&minus;</button>
          <input
            type="range"
            class="zoom-slider"
            aria-label="Zoom"
            min="0"
            max="${j.SLIDER_MAX}"
            step="1"
            .value=${String(t)}
            @input=${this._onSliderInput}
          />
          <button class="zoom-btn" title="Zoom in" @click=${this._onZoomIn}>+</button>
          <input
            type="text"
            class="zoom-input"
            aria-label="Zoom percentage"
            .value=${i}
            @focus=${this._onZoomInputFocus}
            @blur=${this._onZoomInputBlur}
            @keydown=${this._onZoomInputKeydown}
            @input=${this._onZoomInputChange}
          />
          <button
            class="fullscreen-btn"
            title=${this._isFullscreen?"Exit fullscreen":"Fullscreen"}
            @click=${this._toggleFullscreen}
          >
            ${this._isFullscreen?g`<svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M5 2v3H2M14 5h-3V2M11 14v-3h3M2 11h3v3"/></svg>`:g`<svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 6V2h4M10 2h4v4M14 10v4h-4M6 14H2v-4"/></svg>`}
          </button>
        </div>
      </div>
    `}};j.MIN_ZOOM=.1;j.MAX_ZOOM=10;j.ZOOM_STEP=1.1;j.SLIDER_MAX=1e3;j.styles=yt`
    :host {
      display: block;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 0.8125rem;
      color: #ddd;
      user-select: none;
    }

    .section {
      border-bottom: 1px solid #444;
    }

    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 6px 8px;
      border-bottom: 1px solid #444;
    }

    .header-title {
      font-weight: 500;
      font-size: 0.75rem;
      color: #ccc;
    }

    .minimap-container {
      padding: 6px;
    }

    canvas {
      display: block;
      width: 100%;
      border-radius: 3px;
      background: #3a3a3a;
      cursor: crosshair;
    }

    .zoom-controls {
      display: flex;
      align-items: center;
      gap: 4px;
      padding: 4px 6px 6px;
    }

    .zoom-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      border: 1px solid #555;
      border-radius: 4px;
      background: #444;
      color: #ccc;
      cursor: pointer;
      font-size: 14px;
      padding: 0;
      line-height: 1;
      flex-shrink: 0;
    }

    .zoom-btn:hover {
      background: #555;
      color: #fff;
    }

    .zoom-slider {
      flex: 1;
      height: 4px;
      -webkit-appearance: none;
      appearance: none;
      background: #555;
      border-radius: 2px;
      outline: 2px solid transparent;
      min-width: 0;
    }

    .zoom-slider::-webkit-slider-thumb {
      -webkit-appearance: none;
      width: 12px;
      height: 12px;
      border-radius: 50%;
      background: #bbb;
      border: 1px solid #888;
      cursor: grab;
    }

    .zoom-slider::-webkit-slider-thumb:active {
      cursor: grabbing;
    }

    .zoom-input {
      width: 38px;
      background: #333;
      border: 1px solid #555;
      border-radius: 3px;
      padding: 1px 3px;
      text-align: center;
      font-size: 0.6875rem;
      color: #ddd;
      font-family: inherit;
      flex-shrink: 0;
    }

    .zoom-input:focus {
      outline: 1px solid #007bff;
      border-color: #007bff;
    }

    .fullscreen-btn {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      border: 1px solid #555;
      border-radius: 4px;
      background: #444;
      color: #ccc;
      cursor: pointer;
      padding: 0;
      flex-shrink: 0;
    }

    .fullscreen-btn:hover {
      background: #555;
      color: #fff;
    }
  `;me([M()],j.prototype,"_isFullscreen",2);me([M()],j.prototype,"_editingZoom",2);me([M()],j.prototype,"_zoomInputValue",2);j=me([wt("navigator-panel")],j);var no=Object.defineProperty,co=Object.getOwnPropertyDescriptor,z=(e,t,s,i)=>{for(var a=i>1?void 0:i?co(t,s):t,o=e.length-1,r;o>=0;o--)(r=e[o])&&(a=(i?r(t,s,a):r(a))||a);return i&&a&&no(t,s,a),a};const lo=768,ho=800;function po(e,t){return t?e<=ho:e<lo}let D=class extends F{constructor(){super(),this._layerCounter=0,this._canUndo=!1,this._canRedo=!1,this._saving=!1,this._viewportZoom=1,this._viewportPanX=0,this._viewportPanY=0,this._viewportWidth=800,this._viewportHeight=600,this._currentProject=null,this._projectList=[],this._isMobile=!1,this._mobileObserver=null,this.embedded=!1,this._storageState="loading",this._ownsBackend=!1,this._ready=new Promise((t,s)=>{this._resolveReady=t,this._rejectReady=s}),this._savedHistoryTop=null,this._lastReportedModified=!1,this._dirty=!1,this._saveTimer=null,this._saveInProgress=!1,this._savePromise=null,this._saveRequested=!1,this._forceFlushNextSave=!1,this._dirtyVersion=0,this._contentVersion=0,this._savedContentVersion=-1,this._savedHistory=new Map,this._nextHistoryRecordIndex=0,this._historyNeedsRewrite=!1,this._savedLayerBlobs=new Map,this._trackedProjectId=null,this._trackingGeneration=0,this._projectLoads=0,this._onBeforeUnload=t=>{this.canvas?.clearSelection(),this._dirty&&(this._flushPendingSave(),t.preventDefault())},this._onVisibilityChange=()=>{document.hidden&&(this.canvas?.clearSelection(),this._dirty&&this._flushPendingSave())},this._onKeyDown=t=>{if(this._isTextEntryTarget(t))return;if(t.key==="Escape"&&this.canvas?.hasExternalFloat){t.preventDefault(),this.canvas.cancelExternalFloat();return}if(t.key==="Escape"&&this.canvas?.isTransformActive()){t.preventDefault(),this.canvas.cancelTransform();return}if(t.key==="Enter"&&this.canvas?.isTransformActive()){t.preventDefault(),this.canvas.commitTransform();return}const s=t.ctrlKey||t.metaKey,i=t.key.toLowerCase();if(s&&i==="s"&&this.embedded){t.preventDefault(),this._requestSave();return}if(s&&i==="t"){t.preventDefault(),this.canvas?.enterTransformMode();return}if(s&&i==="z"&&!t.shiftKey)t.preventDefault(),this.canvas?.undo();else if(s&&(i==="y"||i==="z"&&t.shiftKey))t.preventDefault(),this.canvas?.redo();else if(s&&i==="c")t.preventDefault(),this.canvas?.copySelection();else if(s&&i==="x")t.preventDefault(),this.canvas?.cutSelection();else if(s&&i==="v")t.preventDefault(),this.canvas?.paste();else if(s&&i==="d")t.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty()),this.canvas?.duplicateInPlace();else if((t.key==="Delete"||t.key==="Backspace")&&(this._state.activeTool==="select"||this._state.activeTool==="stamp"))t.preventDefault(),this.canvas?.deleteSelection();else if(t.key==="Enter"&&this._state.activeTool==="crop"&&this.canvas?.hasCropRect)t.preventDefault(),this.canvas.commitCrop();else if(t.key==="Escape")this._state.activeTool==="crop"&&this.canvas?.hasCropRect?this.canvas.cancelCrop():this.canvas?.hasExternalFloat?this.canvas.cancelExternalFloat():this.canvas?.clearSelection();else if(s&&i==="a"&&t.shiftKey)t.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty()),this.canvas?.selectAllCanvas();else if(s&&i==="a"&&!t.shiftKey)t.preventDefault(),this._state.activeTool!=="select"&&(this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:"select"},this._markDirty()),this.canvas?.selectAll();else if(t.key==="0"&&s)t.preventDefault(),this.canvas?.zoomToFit();else if(s&&(t.key==="="||t.key==="+"))t.preventDefault(),this.canvas?.zoomIn();else if(s&&t.key==="-")t.preventDefault(),this.canvas?.zoomOut();else if(!s&&!t.altKey&&(i==="["||i==="]")){if(t.preventDefault(),this._state.activeTool==="stamp"){const n=this._state.stampSize,c=i==="]"?Math.max(n+1,Math.round(n*1.1)):Math.min(n-1,Math.round(n/1.1));this._updateStampSize(c);return}const a=this._state.brush.size,o=150,r=1;if(i==="]"){const n=Math.min(o,Math.max(a+1,Math.round(a*1.1)));this._updateBrush({size:n})}else{const n=Math.max(r,Math.min(a-1,Math.round(a/1.1)));this._updateBrush({size:n})}}else if(!s&&!t.altKey&&(t.key==="{"||t.key==="}")){t.preventDefault();const a=this._state.brush.hardness;t.key==="}"?this._updateBrush({hardness:Math.round(Math.min(1,a+.1)*10)/10}):this._updateBrush({hardness:Math.round(Math.max(0,a-.1)*10)/10})}else if(!s&&!t.altKey&&!t.shiftKey&&i.length===1){const a=ea(i);a&&a!==this._state.activeTool&&(t.preventDefault(),this.canvas?.cancelCrop(),this.canvas?.clearSelection(),this._state={...this._state,activeTool:a},this._markDirty())}},this._ready.catch(()=>{});const e=this._createLayer(800,600);this._state={activeTool:"pencil",strokeColor:"#000000",fillColor:"#ff0000",useFill:!1,brush:ie(),activePreset:"round",isPresetModified:!1,stampImage:null,activeStampId:null,stampSize:Ie,layers:[e],activeLayerId:e.id,layersPanelOpen:!0,documentWidth:800,documentHeight:600,cropAspectRatio:"free",fontFamily:"sans-serif",fontSize:24,fontBold:!1,fontItalic:!1,eyedropperSampleAll:!0,childMode:!1},this._provider=new ye(this,{context:Lt,initialValue:this._buildContextValue()})}_createLayer(e,t){this._layerCounter++;const s=document.createElement("canvas");return s.width=e,s.height=t,{id:crypto.randomUUID(),name:`Layer ${this._layerCounter}`,visible:!0,opacity:1,blendMode:"normal",canvas:s}}_snapshotLayer(e){const t=e.canvas.getContext("2d");return{id:e.id,name:e.name,visible:e.visible,opacity:e.opacity,blendMode:e.blendMode,imageData:t.getImageData(0,0,e.canvas.width,e.canvas.height)}}_snapshotAllLayers(){return this._state.layers.map(e=>this._snapshotLayer(e))}_compositeLayers(e,t="#ffffff"){const s=this._state.documentWidth,i=this._state.documentHeight,a=document.createElement("canvas");a.width=s,a.height=i;const o=a.getContext("2d");t&&(o.fillStyle=t,o.fillRect(0,0,s,i));for(const r of e)o.globalAlpha=r.opacity,o.globalCompositeOperation=Yt(r.blendMode),o.drawImage(r.canvas,0,0),o.globalCompositeOperation="source-over";return o.globalAlpha=1,a}_flushPendingSave(){this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),this._save(!0)}async _flushPendingSaveAndWait(){this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),await this._save(!0)}_markDirty(e=!1){e||this._contentVersion++,this._dirty=!0,this._dirtyVersion++,this._saveRequested=!0,this._saveTimer&&clearTimeout(this._saveTimer),this._saveTimer=setTimeout(()=>{this._save()},500)}_updateBrush(e){this._state={...this._state,brush:{...this._state.brush,...e},isPresetModified:!0},this._markDirty()}_updateStampSize(e){const t=rs(e,this._state.stampSize);t!==this._state.stampSize&&(this._state={...this._state,stampSize:t},this._markDirty())}_planHistorySave(e,t){const s=this._savedHistory;let i=this._historyNeedsRewrite||this._trackedProjectId!==e||!this._backend?.history.updateEntries;if(!i){let o=-1,r=!1;for(const n of t){const c=s.get(n);if(!c)r=!0;else if(r||c.index<=o){i=!0;break}else o=c.index}}if(i)return{rewrite:i,remove:[...s],add:t,firstIndex:0};const a=new Set(t);return{rewrite:i,remove:[...s].filter(([o])=>!a.has(o)),add:t.filter(o=>!s.has(o)),firstIndex:this._nextHistoryRecordIndex}}_recordSavedHistory(e,t,s){this._trackedProjectId=e,t.rewrite&&this._savedHistory.clear();for(const[i]of t.remove)this._savedHistory.delete(i);t.add.forEach((i,a)=>{const o=new Set;Dt(s[a].entry,o),this._savedHistory.set(i,{index:s[a].index,blobRefs:[...o]})}),this._nextHistoryRecordIndex=t.firstIndex+t.add.length,this._historyNeedsRewrite=!1}_trackLoadedProject(e,t,s,i=new Map){this._trackedProjectId=e,this._trackingGeneration++,this._savedHistory=new Map(t.map((a,o)=>{const r=new Set;return Dt(s[o].entry,r),[a,{index:s[o].index,blobRefs:[...r]}]})),this._nextHistoryRecordIndex=s.reduce((a,o)=>Math.max(a,o.index+1),0),this._historyNeedsRewrite=!1,this._savedLayerBlobs=i,this._savedContentVersion=-1}async _enterProject(e,t){this._projectLoads++;try{this._currentProject=e,await t()}finally{this._projectLoads--}}_renderThumbnail(e){const t=Math.min(1,D.THUMBNAIL_SIZE/Math.max(e.width,e.height,1)),s=document.createElement("canvas");return s.width=Math.max(1,Math.round(e.width*t)),s.height=Math.max(1,Math.round(e.height*t)),s.getContext("2d").drawImage(e,0,0,s.width,s.height),s}async _save(e=!1){if(this._savePromise)return e&&(this._forceFlushNextSave=!0),this._dirty&&(this._saveRequested=!0),this._savePromise;if(!(!this._currentProject||!this._dirty||this._projectLoads>0)&&this._backend){this._savePromise=(async()=>{this._saveInProgress=!0,this._saving=!0;let t=e;try{for(;this._currentProject&&this._dirty&&this._projectLoads===0;){const s=this._currentProject.id,i=this._dirtyVersion,a=this._contentVersion,o=Date.now(),r=this._forceFlushNextSave;this._forceFlushNextSave=!1,this._saveRequested=!1;const n=t||r,c={activeTool:this._state.activeTool,strokeColor:this._state.strokeColor,fillColor:this._state.fillColor,useFill:this._state.useFill,brushSize:this._state.brush.size,stampSize:this._state.stampSize,opacity:this._state.brush.opacity,flow:this._state.brush.flow,hardness:this._state.brush.hardness,spacing:this._state.brush.spacing,pressureSize:this._state.brush.pressureSize,pressureOpacity:this._state.brush.pressureOpacity,pressureCurve:this._state.brush.pressureCurve,tip:{...this._state.brush.tip},ink:{...this._state.brush.ink},activePreset:this._state.activePreset,isPresetModified:this._state.isPresetModified,cropAspectRatio:this._state.cropAspectRatio,fontFamily:this._state.fontFamily,fontSize:this._state.fontSize,fontBold:this._state.fontBold,fontItalic:this._state.fontItalic,eyedropperSampleAll:this._state.eyedropperSampleAll,childMode:this._state.childMode},l=this._state.documentWidth,h=this._state.documentHeight,d=this._state.activeLayerId,p=this._state.layersPanelOpen,f=this.canvas?.getFloatSnapshot()??null,u=!f&&a===this._savedContentVersion&&this._trackedProjectId===s&&this._state.layers.every(w=>this._savedLayerBlobs.has(w.id)),_=this._state.layers.map(w=>{const L={id:w.id,name:w.name,visible:w.visible,opacity:w.opacity,blendMode:w.blendMode};if(u)return{...L,imageData:null};const Fe=w.canvas.getContext("2d").getImageData(0,0,w.canvas.width,w.canvas.height);if(f&&w.id===f.layerId){const At=document.createElement("canvas");At.width=w.canvas.width,At.height=w.canvas.height;const ve=At.getContext("2d");return ve.putImageData(Fe,0,0),ve.drawImage(f.tempCanvas,f.x,f.y),{...L,imageData:ve.getImageData(0,0,At.width,At.height)}}return{...L,imageData:Fe}}),m=_.map(w=>w.imageData?as(w.imageData):this._savedLayerBlobs.get(w.id).hash),v=this.canvas?.getViewport()??{zoom:1,panX:0,panY:0},b=this.canvas?.getHistory()??[],y=this.canvas?.getHistoryIndex()??-1,x=this._trackingGeneration,C=this._planHistorySave(s,b),k=C.rewrite,P=this._backend.blobs,[R,E,H]=await Promise.all([this._backend.state.get(s),this._backend.projects.get(s),k?this._backend.history.getEntries(s):Promise.resolve([])]);if(!E)break;const B=R?.layers.map(w=>w.imageBlobRef)??[],it=E.thumbnailRef??null,Q=[],at={get:w=>P.get(w),delete:w=>P.delete(w),deleteMany:w=>P.deleteMany(w),gc:P.gc?w=>P.gc(w):void 0,async put(w){const L=await P.put(w);return Q.push(L),L}};let Kt,xt;try{Kt=await Promise.all(_.map((w,L)=>{const W=this._savedLayerBlobs.get(w.id);return W&&W.hash===m[L]&&B.includes(W.blobRef)?{id:w.id,name:w.name,visible:w.visible,opacity:w.opacity,blendMode:w.blendMode,imageBlobRef:W.blobRef}:Ki(w,w.imageData,at)})),xt=await Promise.all(C.add.map(async(w,L)=>({projectId:s,index:C.firstIndex+L,entry:await Ji(w,at)})))}catch(w){throw Q.length>0&&P.deleteMany(Q).catch(()=>{}),w}const qs={projectId:s,toolSettings:c,canvasWidth:l,canvasHeight:h,layers:Kt,activeLayerId:d,layersPanelOpen:p,historyIndex:y,zoom:v.zoom,panX:v.panX,panY:v.panY};let ge=null;if(this.canvas?.mainCanvas)try{ge=await Ye(this._renderThumbnail(this.canvas.mainCanvas))}catch{}try{await this._backend.state.save(qs),k?await this._backend.history.replaceAll(s,xt):(C.remove.length>0||xt.length>0)&&await this._backend.history.updateEntries(s,C.remove.map(([,w])=>w.index),xt)}catch(w){throw R&&this._backend.state.save(R).catch(L=>{console.error("Failed to rollback state after save failure:",L)}),P.deleteMany(Q).catch(()=>{}),w}this._currentProject?.id===s&&this._trackingGeneration===x&&(this._recordSavedHistory(s,C,xt),this._savedLayerBlobs=new Map(_.map((w,L)=>[w.id,{hash:m[L],blobRef:Kt[L].imageBlobRef}])),this._savedContentVersion=a);let Ct=it;try{ge?(Ct=await P.put(ge),await this._backend.projects.update(s,{thumbnailRef:Ct})):await this._backend.projects.update(s,{})}catch{Ct!==it&&Ct&&P.delete(Ct).catch(()=>{})}const Zs=new Set(Kt.map(w=>w.imageBlobRef)),zt=B.filter(w=>!Zs.has(w));if(it&&it!==Ct&&zt.push(it),!k)for(const[,w]of C.remove)zt.push(...w.blobRefs);if(k&&H.length>0){const w=new Set;for(const W of H)Dt(W.entry,w);const L=new Set;for(const W of xt)Dt(W.entry,L);for(const W of w)L.has(W)||zt.push(W)}if(zt.length>0&&P.deleteMany(zt).catch(()=>{}),this._currentProject?.id===s&&this._dirtyVersion===i&&(this._dirty=!1),this._projectList=await this._backend.projects.list(),!n){const w=Date.now()-o;w<1500&&await new Promise(L=>setTimeout(L,1500-w))}if(!this._saveRequested||!this._dirty)break;t=!1}}catch(s){s instanceof Es?console.error("Storage quota exceeded. Consider deleting old projects to free space."):console.error("Save failed:",s)}finally{this._saving=!1,this._saveInProgress=!1}})();try{await this._savePromise}finally{this._savePromise=null}}}_isTextEntryTarget(e){for(const t of e.composedPath())if(t instanceof HTMLElement){if(t.isContentEditable||t instanceof HTMLTextAreaElement)return!0;if(t instanceof HTMLInputElement)return!D.NON_TEXT_INPUT_TYPES.has(t.type);if(t instanceof HTMLDialogElement&&t.open)return!0}return!1}_onCommitOpacity(e){const{layerId:t,before:s,after:i}=e.detail;this.canvas?.pushLayerOperation({type:"opacity",layerId:t,before:s,after:i}),this._markDirty()}_onCropCommit(e){const{width:t,height:s}=e.detail;this._applyDocumentDimensions(t,s),this._state={...this._state,layers:[...this._state.layers]},this._markDirty()}async _resetToFreshProject(e=800,t=600,s="#ffffff"){this.canvas?.clearSelection(),this._layerCounter=0;const i=e,a=t,o=this._createLayer(i,a);if(this._state={activeTool:"pencil",strokeColor:"#000000",fillColor:"#ff0000",useFill:!1,brush:ie(),activePreset:"round",isPresetModified:!1,stampImage:null,activeStampId:null,stampSize:Ie,layers:[o],activeLayerId:o.id,layersPanelOpen:!0,documentWidth:i,documentHeight:a,cropAspectRatio:"free",fontFamily:"sans-serif",fontSize:24,fontBold:!1,fontItalic:!1,eyedropperSampleAll:!0,childMode:!1},await this.updateComplete,this.canvas?.setHistory([],-1),s){const r=o.canvas.getContext("2d");r.fillStyle=s,r.fillRect(0,0,o.canvas.width,o.canvas.height)}this.canvas?.composite(),this._dirty=!1,this._trackLoadedProject(this._currentProject?.id??null,[],[]),this._historyNeedsRewrite=!0}async _loadProject(e){try{this.canvas?.clearSelection();const t=await this._backend.state.get(e);if(!t){await this._resetToFreshProject();return}const s=16384;if(t.canvasWidth<=0||t.canvasWidth>s||t.canvasHeight<=0||t.canvasHeight>s){console.error("Invalid canvas dimensions in saved state:",t.canvasWidth,t.canvasHeight),await this._resetToFreshProject();return}const i=this._backend.blobs,a=await Promise.all(t.layers.map(f=>Gi(f,t.canvasWidth,t.canvasHeight,i)));if(a.length===0){await this._resetToFreshProject();return}const o=await this._backend.history.getEntries(e),r=await Promise.all(o.map(f=>Qi(f.entry,i))),n=new Map(a.map((f,u)=>{const _=f.canvas.getContext("2d").getImageData(0,0,f.canvas.width,f.canvas.height);return[f.id,{hash:as(_),blobRef:t.layers[u].imageBlobRef}]})),c=a.reduce((f,u)=>{const _=u.name.match(/^Layer (\d+)$/);return _?Math.max(f,parseInt(_[1])):f},0);this._layerCounter=c;const l=a.some(f=>f.id===t.activeLayerId)?t.activeLayerId:a[0].id,h=t.toolSettings,d=ie(),p={size:h.brushSize??d.size,opacity:h.opacity??d.opacity,flow:h.flow??d.flow,hardness:h.hardness??d.hardness,spacing:h.spacing??d.spacing,pressureSize:h.pressureSize??d.pressureSize,pressureOpacity:h.pressureOpacity??d.pressureOpacity,pressureCurve:h.pressureCurve??d.pressureCurve,tip:{...d.tip,...h.tip??{}},ink:{...d.ink,...h.ink??{}}};this._state={activeTool:h.activeTool==="marker"?"pencil":h.activeTool,strokeColor:h.strokeColor,fillColor:h.fillColor,useFill:h.useFill,brush:p,activePreset:h.activePreset??"round",isPresetModified:h.isPresetModified??!1,stampImage:null,activeStampId:null,stampSize:rs(h.stampSize),layers:a,activeLayerId:l,layersPanelOpen:t.layersPanelOpen,documentWidth:t.canvasWidth,documentHeight:t.canvasHeight,cropAspectRatio:h.cropAspectRatio??"free",fontFamily:h.fontFamily??"sans-serif",fontSize:h.fontSize??24,fontBold:h.fontBold??!1,fontItalic:h.fontItalic??!1,eyedropperSampleAll:h.eyedropperSampleAll??!0,childMode:h.childMode??!1},await this.updateComplete,this.canvas?.setHistory(r,t.historyIndex??r.length-1),this._dirty=!1,this._trackLoadedProject(e,r,o,n),t.zoom!=null&&t.panX!=null&&t.panY!=null?this.canvas?.setViewport(t.zoom,t.panX,t.panY):(this.canvas?.centerDocument(),this.canvas?.composite())}catch(t){console.error("Failed to load project:",t),await this._resetToFreshProject()}}_applyDocumentDimensions(e,t){this._state={...this._state,documentWidth:e,documentHeight:t}}_buildContextValue(){return{state:this._state,setTool:e=>{this._state.activeTool!==e&&(this.canvas?.isTransformActive()&&this.canvas.commitTransform(),this.canvas?.cancelCrop(),this.canvas?.clearSelection()),this._state={...this._state,activeTool:e},this._markDirty()},setStrokeColor:e=>{this._state={...this._state,strokeColor:e},this._markDirty()},setFillColor:e=>{this._state={...this._state,fillColor:e},this._markDirty()},setUseFill:e=>{this._state={...this._state,useFill:e},this._markDirty()},setBrushSize:e=>{const t=Number.isNaN(e)?this._state.brush.size:e;this._updateBrush({size:Math.max(1,Math.min(150,t))})},setStampSize:e=>{this._updateStampSize(e)},setStampImage:(e,t=null)=>{this._state={...this._state,stampImage:e,activeStampId:e?t:null},this._markDirty()},undo:()=>this.canvas?.undo(),redo:()=>this.canvas?.redo(),clearCanvas:()=>this.canvas?.clearCanvas(),saveCanvas:()=>this._requestSave(),embedded:this.embedded,addLayer:e=>{this.canvas?.clearSelection();const t=this._createLayer(this._state.documentWidth,this._state.documentHeight);e&&(t.name=e,this._layerCounter--);const i=this._state.layers.findIndex(o=>o.id===this._state.activeLayerId)+1,a=[...this._state.layers];return a.splice(i,0,t),this._state={...this._state,layers:a,activeLayerId:t.id},this.canvas?.pushLayerOperation({type:"add-layer",layer:this._snapshotLayer(t),index:i}),this._markDirty(),t.id},deleteLayer:e=>{if(this._state.layers.length<=1)return;const t=this._state.layers.findIndex(r=>r.id===e);if(t===-1)return;e===this._state.activeLayerId&&this.canvas?.clearSelection();const s=this._state.layers[t],i=this._snapshotLayer(s),a=this._state.layers.filter(r=>r.id!==e),o=this._state.activeLayerId===e?a[Math.min(t,a.length-1)].id:this._state.activeLayerId;this._state={...this._state,layers:a,activeLayerId:o},this.canvas?.pushLayerOperation({type:"delete-layer",layer:i,index:t}),this._markDirty()},setActiveLayer:e=>{this._state.layers.some(t=>t.id===e)&&e!==this._state.activeLayerId&&(this.canvas?.clearSelection(),this._state={...this._state,activeLayerId:e},this._markDirty())},setLayerVisibility:(e,t)=>{const s=this._state.layers.find(o=>o.id===e);if(!s||s.visible===t)return;const i=s.visible,a=this._state.layers.map(o=>o.id===e?{...o,visible:t}:o);this._state={...this._state,layers:a},this.canvas?.pushLayerOperation({type:"visibility",layerId:e,before:i,after:t}),this._markDirty()},setLayerOpacity:(e,t)=>{if(!this._state.layers.find(r=>r.id===e))return;const i=Number.isFinite(t)?t:1,a=Math.max(0,Math.min(1,i)),o=this._state.layers.map(r=>r.id===e?{...r,opacity:a}:r);this._state={...this._state,layers:o},this._markDirty()},reorderLayer:(e,t)=>{const s=this._state.layers.findIndex(r=>r.id===e);if(s===-1||s===t)return;const i=[...this._state.layers],[a]=i.splice(s,1),o=t<0?Math.max(i.length+t,0):Math.min(t,i.length);i.splice(o,0,a),this._state={...this._state,layers:i},this.canvas?.pushLayerOperation({type:"reorder",fromIndex:s,toIndex:o}),this._markDirty()},renameLayer:(e,t)=>{const s=this._state.layers.find(o=>o.id===e);if(!s||s.name===t)return;const i=s.name,a=this._state.layers.map(o=>o.id===e?{...o,name:t}:o);this._state={...this._state,layers:a},this.canvas?.pushLayerOperation({type:"rename",layerId:e,before:i,after:t}),this._markDirty()},setLayerBlendMode:(e,t)=>{const s=this._state.layers.find(o=>o.id===e);if(!s||s.blendMode===t)return;const i=s.blendMode,a=this._state.layers.map(o=>o.id===e?{...o,blendMode:t}:o);this._state={...this._state,layers:a},this.canvas?.pushLayerOperation({type:"blend-mode",layerId:e,before:i,after:t}),this._markDirty()},mergeLayerDown:e=>{const t=this._state.layers,s=t.findIndex(h=>h.id===e);if(s<=0)return;this.canvas?.clearSelection();const i=this._snapshotAllLayers(),a=this._state.activeLayerId,o=t[s-1],r=t[s],n=this._compositeLayers([o,r],null),c=t.filter(h=>h.id!==r.id).map(h=>h.id===o.id?{...h,canvas:n,opacity:1,blendMode:"normal"}:h);this._state={...this._state,layers:c,activeLayerId:o.id};const l=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:i,afterLayers:l,previousActiveLayerId:a,afterActiveLayerId:o.id}),this._markDirty()},mergeVisibleLayers:()=>{const e=this._state.layers,t=e.filter(d=>d.visible);if(t.length<2)return;this.canvas?.clearSelection();const s=this._snapshotAllLayers(),i=this._state.activeLayerId,a=t[0],o=this._compositeLayers(t,null),r=new Set(t.map(d=>d.id)),n=e.filter(d=>!r.has(d.id)||d.id===a.id).map(d=>d.id===a.id?{...d,canvas:o,opacity:1,blendMode:"normal"}:d),l=n.some(d=>d.id===i)?i:a.id;this._state={...this._state,layers:n,activeLayerId:l};const h=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:s,afterLayers:h,previousActiveLayerId:i,afterActiveLayerId:l}),this._markDirty()},flattenImage:()=>{if(this._state.layers.length<=1)return;this.canvas?.clearSelection();const e=this._snapshotAllLayers(),t=this._state.activeLayerId,s=this._state.layers.filter(n=>n.visible),i=s.length>0?s[0]:this._state.layers[0],a=this._compositeLayers(s),o={id:i.id,name:i.name,visible:!0,opacity:1,blendMode:"normal",canvas:a};this._state={...this._state,layers:[o],activeLayerId:o.id};const r=this._snapshotAllLayers();this.canvas?.pushLayerOperation({type:"merge",beforeLayers:e,afterLayers:r,previousActiveLayerId:t,afterActiveLayerId:o.id}),this._markDirty()},toggleLayersPanel:()=>{this._state={...this._state,layersPanelOpen:!this._state.layersPanelOpen},this._markDirty()},setCropAspectRatio:e=>{this._state={...this._state,cropAspectRatio:e},this._markDirty()},setFontFamily:e=>{this._state={...this._state,fontFamily:e},this._markDirty()},setFontSize:e=>{const t=Number.isFinite(e)?e:8;this._state={...this._state,fontSize:Math.max(8,Math.min(200,t))},this._markDirty()},setFontBold:e=>{this._state={...this._state,fontBold:e},this._markDirty()},setFontItalic:e=>{this._state={...this._state,fontItalic:e},this._markDirty()},setBrush:e=>{this._updateBrush(e)},setBrushTip:e=>{this._updateBrush({tip:{...this._state.brush.tip,...e}})},setBrushInk:e=>{this._updateBrush({ink:{...this._state.brush.ink,...e}})},selectPreset:e=>{const t=Ii(e);if(!t)return;const s=t.descriptor;this._state={...this._state,brush:{...s,tip:{...s.tip},ink:{...s.ink}},activePreset:e,isPresetModified:!1},this._markDirty()},setEyedropperSampleAll:e=>{this._state={...this._state,eyedropperSampleAll:e},this._markDirty()},canUndo:this._canUndo,canRedo:this._canRedo,currentProject:this._currentProject,projectList:this._projectList,saving:this._saving,zoom:this._viewportZoom,panX:this._viewportPanX,panY:this._viewportPanY,viewportWidth:this._viewportWidth,viewportHeight:this._viewportHeight,isMobile:this._isMobile,switchProject:e=>{if(e===this._currentProject?.id)return;(async()=>{this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait();const s=this._projectList.find(i=>i.id===e);s&&await this._enterProject(s,()=>this._loadProject(e))})().catch(s=>console.error("Switch project failed:",s))},createProject:(e,t,s)=>{(async()=>{this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait();const a=await this._backend.projects.create({name:e,thumbnailRef:null});await this._enterProject(a,async()=>{this._projectList=await this._backend.projects.list(),await this._resetToFreshProject(t,s)}),this._markDirty()})().catch(a=>console.error("Create project failed:",a))},deleteProject:e=>{(async()=>{if(this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait(),await this._projectService.deleteProject(e),this._projectList=await this._backend.projects.list(),e===this._currentProject?.id)if(this._projectList.length>0){const s=this._projectList[0];await this._enterProject(s,()=>this._loadProject(s.id))}else{const s=await this._backend.projects.create({name:"Untitled",thumbnailRef:null});this._projectList=[s],await this._enterProject(s,()=>this._resetToFreshProject()),this._markDirty()}})().catch(s=>console.error("Delete project failed:",s))},renameProject:(e,t)=>{(async()=>{const i=await this._backend.projects.update(e,{name:t});this._currentProject?.id===e&&(this._currentProject=i),this._projectList=await this._backend.projects.list()})().catch(i=>console.error("Rename project failed:",i))},transformActive:this.canvas?.isTransformActive()??!1,getTransformValues:()=>this.canvas?.getTransformValues()??null,setTransformValue:(e,t)=>this.canvas?.setTransformValue(e,t),setChildMode:e=>{this._state={...this._state,childMode:e},e&&!sa.has(this._state.activeTool)&&(this._state={...this._state,activeTool:"pencil"}),this._markDirty()}}}willUpdate(){this._provider.setValue(this._buildContextValue()),this.toggleAttribute("mobile",this._isMobile)}_onHistoryChange(e){this._canUndo=e.detail.canUndo,this._canRedo=e.detail.canRedo,this._markDirty(),this._reportModified()}whenReady(){return this._ready}async openImage(e,t={}){await this._ready;const s=await createImageBitmap(e);try{await this._replaceDocument(s.width,s.height,null,t.name??"Untitled"),this._state.layers[0].canvas.getContext("2d").drawImage(s,0,0)}finally{s.close()}await this._showWholeDocument(),this._markDirty(),this._markSaved()}async newDocument(e,t,s={}){await this._ready,await this._replaceDocument(e,t,s.background===void 0?"#ffffff":s.background,s.name??"Untitled"),await this._showWholeDocument(),this._markDirty(),this._markSaved()}async exportImage(e={}){await this._ready;const t=e.type??"image/png",s=e.background!==void 0?e.background:t==="image/jpeg"?"#ffffff":null,i=this.canvas.renderFlattened(s);return new Promise((a,o)=>{i.toBlob(r=>{r?a(r):o(new Error(`Could not encode the image as ${t}`))},t,e.quality)})}get modified(){return this.canvas?this._historyTop()!==this._savedHistoryTop||this.canvas.isTransformActive():!1}markSaved(){this._markSaved()}async _showWholeDocument(){await this.updateComplete;const e=this.canvas;if(!e)return;await e.updateComplete,this._state.documentWidth<=e.clientWidth&&this._state.documentHeight<=e.clientHeight?e.centerDocument():e.zoomToFit(),e.composite()}_historyTop(){const e=this.canvas?.getHistoryIndex()??-1;return e>=0?this.canvas.getHistory()[e]??null:null}_markSaved(){this._savedHistoryTop=this._historyTop(),this._reportModified()}_reportModified(){const e=this.modified;e!==this._lastReportedModified&&(this._lastReportedModified=e,this.dispatchEvent(new CustomEvent("modified-change",{detail:{modified:e},bubbles:!0,composed:!0})))}_requestSave(){if(!this.embedded){this.canvas?.saveCanvas();return}this.canvas?.clearSelection(),this.dispatchEvent(new CustomEvent("save-request",{bubbles:!0,composed:!0}))}async _replaceDocument(e,t,s,i){if(!(e>0&&t>0&&e<=16384&&t<=16384))throw new RangeError(`Document size ${e}×${t} is outside 1–16384 pixels`);this.canvas?.cancelCrop(),this.canvas?.clearSelection(),(this._savePromise||this._dirty)&&await this._flushPendingSaveAndWait();const o=this._currentProject,r=await this._backend.projects.create({name:i,thumbnailRef:null});await this._enterProject(r,async()=>{await this._resetToFreshProject(Math.round(e),Math.round(t),s)}),this.embedded&&o&&await this._projectService.deleteProject(o.id),this._projectList=await this._backend.projects.list()}_onViewportChange(){if(this.canvas){const e=this.canvas.getViewport();this._viewportZoom=e.zoom,this._viewportPanX=e.panX,this._viewportPanY=e.panY,this._viewportWidth=this.canvas.clientWidth,this._viewportHeight=this.canvas.clientHeight}this._markDirty(!0)}_onTransformChange(){this.requestUpdate()}_onNavigatorPan(e){if(!this.canvas)return;const{panX:t,panY:s}=e.detail,i=this.canvas.getViewport();this.canvas.setViewport(i.zoom,t,s)}_onNavigatorZoom(e){if(!this.canvas)return;const t=e.detail.zoom,s=this.canvas.getViewport(),i=this.canvas.clientWidth/2,a=this.canvas.clientHeight/2,o=(i-s.panX)/s.zoom,r=(a-s.panY)/s.zoom,n=i-o*t,c=a-r*t;this.canvas.setViewport(t,n,c)}_onLayerUndo(e){const t=e.detail;switch(t.action){case"remove-layer":{const s=this._state.layers.findIndex(o=>o.id===t.layerId),i=this._state.layers.filter(o=>o.id!==t.layerId);if(i.length===0)return;const a=this._state.activeLayerId===t.layerId?i[Math.min(Math.max(0,s-1),i.length-1)].id:this._state.activeLayerId;this._state={...this._state,layers:i,activeLayerId:a};break}case"restore-layer":{const s=t.snapshot,i=this._state.documentWidth,a=this._state.documentHeight,o=document.createElement("canvas");o.width=i,o.height=a,o.getContext("2d").putImageData(s.imageData,0,0);const r={id:s.id,name:s.name,visible:s.visible,opacity:s.opacity,blendMode:s.blendMode??"normal",canvas:o},n=[...this._state.layers],c=t.index===-1?n.length:t.index;n.splice(c,0,r);const l=n.some(h=>h.id===this._state.activeLayerId);this._state={...this._state,layers:n,activeLayerId:l?this._state.activeLayerId:r.id};break}case"reorder":{const s=[...this._state.layers];if(t.fromIndex<0||t.fromIndex>=s.length||t.toIndex<0||t.toIndex>=s.length)break;const[i]=s.splice(t.fromIndex,1);s.splice(t.toIndex,0,i),this._state={...this._state,layers:s};break}case"refresh":{this._state={...this._state,layers:[...this._state.layers]};break}case"crop-restore":{const s=t.layers,i=t.width,a=t.height,o=this._state.layers.map(r=>{const n=s.find(l=>l.id===r.id);if(!n)return r;const c=document.createElement("canvas");return c.width=n.imageData.width,c.height=n.imageData.height,c.getContext("2d").putImageData(n.imageData,0,0),{...r,canvas:c,visible:n.visible,opacity:n.opacity,blendMode:n.blendMode??"normal",name:n.name}});this._applyDocumentDimensions(i,a),this._state={...this._state,layers:o};break}case"stack-replace":{const s=t.layers,i=t.activeLayerId,a=s.map(o=>{const r=document.createElement("canvas");return r.width=o.imageData.width,r.height=o.imageData.height,r.getContext("2d").putImageData(o.imageData,0,0),{id:o.id,name:o.name,visible:o.visible,opacity:o.opacity,blendMode:o.blendMode??"normal",canvas:r}});this._state={...this._state,layers:a,activeLayerId:i};break}}this._markDirty()}_updateMobileLayout(e){const t=po(e,this._isMobile);t!==this._isMobile&&(this._isMobile=t,!t&&this._state.childMode&&(this._state={...this._state,childMode:!1}))}connectedCallback(){super.connectedCallback(),this._initStorage(),this._mobileObserver=new ResizeObserver(e=>{for(const t of e)this._updateMobileLayout(t.contentRect.width)}),this._mobileObserver.observe(this),this.addEventListener("keydown",this._onKeyDown),window.addEventListener("beforeunload",this._onBeforeUnload),document.addEventListener("visibilitychange",this._onVisibilityChange)}_initStorage(){this._initPromise||(this._initPromise=this._doInitStorage())}async _doInitStorage(){try{const e=!!this.storageBackend,t=this.storageBackend??(this.embedded?new Oi:new Vi);await t.init(),this._backend=t,this._ownsBackend=!e,this._projectService=new Ri(t),this._storageProvider=new ye(this,{context:Ls,initialValue:this._backend}),this._serviceProvider=new ye(this,{context:zs,initialValue:this._projectService}),this._storageState="ready",await this._bootstrapProjects(),this._markSaved(),this._resolveReady()}catch(e){this._rejectReady(e),console.error("Storage initialization failed:",e),this._storageState="error",this._storageError="Could not open local storage. Try reloading or checking browser storage settings."}}async _bootstrapProjects(){if(this._projectList=await this._backend.projects.list(),this._projectList.length>0){const e=this._projectList[0];await this._enterProject(e,()=>this._loadProject(e.id))}else{const e=await this._backend.projects.create({name:"Untitled",thumbnailRef:null});this._currentProject=e,this._projectList=[e],this._markDirty()}}disconnectedCallback(){if(super.disconnectedCallback(),this._mobileObserver?.disconnect(),this._mobileObserver=null,this.removeEventListener("keydown",this._onKeyDown),window.removeEventListener("beforeunload",this._onBeforeUnload),document.removeEventListener("visibilitychange",this._onVisibilityChange),this._dirty||this._savePromise){const e=this._ownsBackend?this._backend:void 0;(this._dirty?this._flushPendingSaveAndWait():this._savePromise).finally(()=>e?.dispose())}else this._saveTimer&&(clearTimeout(this._saveTimer),this._saveTimer=null),this._ownsBackend&&this._backend?.dispose()}render(){return this._storageState==="loading"?g`<div style="display:flex;align-items:center;justify-content:center;height:100%;color:#888;">Loading...</div>`:this._storageState==="error"?g`<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;height:100%;color:#ff6b6b;gap:8px;">
        <p>Failed to initialize storage</p>
        <p style="font-size:0.85em;color:#999;">${this._storageError}</p>
      </div>`:g`
      ${this._isMobile?"":g`<tool-settings></tool-settings>`}
      <div class="main-area">
        <app-toolbar></app-toolbar>
        <drawing-canvas
          @history-change=${this._onHistoryChange}
          @layer-undo=${this._onLayerUndo}
          @crop-commit=${this._onCropCommit}
          @transform-change=${this._onTransformChange}
          @viewport-change=${this._onViewportChange}
        ></drawing-canvas>
        ${this._isMobile?"":g`
          <div class="right-sidebar ${this._state.layersPanelOpen?"":"collapsed"}">
            <navigator-panel
              @navigator-pan=${this._onNavigatorPan}
              @navigator-zoom=${this._onNavigatorZoom}
            ></navigator-panel>
            <layers-panel @commit-opacity=${this._onCommitOpacity}></layers-panel>
          </div>
        `}
      </div>
      ${this._isMobile&&!this._state.childMode?g`<layers-panel @commit-opacity=${this._onCommitOpacity}></layers-panel>`:""}
    `}};D.styles=yt`
    :host {
      display: flex;
      flex-direction: column;
      /* The document's border-box rule does not cross the shadow boundary, so
         set it here: safe-area padding must fit inside the 100% height. */
      box-sizing: border-box;
      width: 100%;
      height: 100%;
      /* Keep the UI clear of the status bar and home indicator when installed
         to the home screen (viewport-fit=cover). */
      padding-top: env(safe-area-inset-top);
      padding-left: env(safe-area-inset-left);
      padding-right: env(safe-area-inset-right);
      background: #1e1e1e;
      font-family: system-ui, -apple-system, sans-serif;
      position: relative;
    }

    /* The mobile toolbar and layers panel pad their own bottom inset. */
    :host(:not([mobile])) {
      padding-bottom: env(safe-area-inset-bottom);
    }

    .main-area {
      display: flex;
      flex: 1;
      min-height: 0;
    }

    drawing-canvas {
      flex: 1;
    }

    .right-sidebar {
      display: flex;
      flex-direction: column;
      overflow: hidden;
      height: 100%;
      width: 200px;
      border-left: 1px solid #444;
      background: #2c2c2c;
      transition: width 0.2s ease;
    }

    .right-sidebar.collapsed {
      width: 32px;
    }

    .right-sidebar layers-panel {
      flex: 1;
      min-height: 0;
    }

    /* ── Mobile layout ─────────────────────────── */
    :host([mobile]) {
      flex-direction: column;
    }


    :host([mobile]) .main-area {
      flex-direction: column;
    }


    :host([mobile]) .main-area app-toolbar {
      order: 1;
    }
  `;D.THUMBNAIL_SIZE=256;D.NON_TEXT_INPUT_TYPES=new Set(["button","checkbox","color","file","hidden","image","radio","range","reset","submit"]);z([M()],D.prototype,"_state",2);z([M()],D.prototype,"_canUndo",2);z([M()],D.prototype,"_canRedo",2);z([M()],D.prototype,"_saving",2);z([M()],D.prototype,"_viewportZoom",2);z([M()],D.prototype,"_viewportPanX",2);z([M()],D.prototype,"_viewportPanY",2);z([M()],D.prototype,"_viewportWidth",2);z([M()],D.prototype,"_viewportHeight",2);z([M()],D.prototype,"_currentProject",2);z([M()],D.prototype,"_projectList",2);z([M()],D.prototype,"_isMobile",2);z([Be({attribute:!1})],D.prototype,"storageBackend",2);z([Be({type:Boolean,reflect:!0})],D.prototype,"embedded",2);z([M()],D.prototype,"_storageState",2);z([M()],D.prototype,"_storageError",2);z([M()],D.prototype,"_backend",2);z([M()],D.prototype,"_projectService",2);z([fe("drawing-canvas")],D.prototype,"canvas",2);D=z([wt("drawing-app")],D);export{Ft as AppToolbar,D as DrawingApp,T as DrawingCanvas,Vi as IndexedDBBackend,Oi as MemoryBackend,O as ToolSettings,Lt as drawingContext};
